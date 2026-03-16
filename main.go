package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gookit/color"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var version = "dev" // overridden by ldflags at release build time

func die(msg string, err error) {
	fmt.Fprintf(os.Stderr, "error: %s: %v\n", msg, err)
	os.Exit(1)
}

type podRow struct {
	namespace  string
	name       string
	ready      string
	readyCount int
	totalCount int
	status     string
	restarts   string
	age        string
	lastReason string
}

func printUsage(kubeconfigDefault string) {
	title := color.Style{color.Bold, color.FgWhite}
	section := color.Style{color.Bold, color.FgCyan}
	flagName := color.FgCyan
	desc := color.FgWhite
	subtle := color.FgGray

	title.Println("Kubernetes Get Pod Status (kgps)")
	subtle.Printf("  version %s\n", version)
	fmt.Println()

	section.Println("USAGE")
	fmt.Println("  kgps [flags]")
	fmt.Println()

	section.Println("FLAGS")

	row := func(names, argument, description, def string) {
		nameCol := flagName.Sprint(names)
		argCol := ""
		if argument != "" {
			argCol = subtle.Sprintf(" <%s>", argument)
		}
		defCol := ""
		if def != "" {
			defCol = subtle.Sprintf("  (default: %s)", def)
		}
		fmt.Printf("  %-38s %s%s\n", nameCol+argCol, desc.Sprint(description), defCol)
	}

	row("-n, -namespace", "namespace", "Namespace to list pods in.", "current context")
	row("-A", "", "List pods across all namespaces.", "")
	row("-w, -watch", "", "Watch for pod changes.", "")
	row("--kubeconfig", "path", "Path to kubeconfig file.", kubeconfigDefault)
	row("-v, -version", "", "Print version and exit.", "")
	row("-h, -help", "", "Show this help message.", "")
	fmt.Println()
}

func main() {
	var kubeconfig *string
	kubeconfigDefault := "~/.kube/config"
	if env := os.Getenv("KUBECONFIG"); env != "" {
		kubeconfig = flag.String("kubeconfig", env, "")
		kubeconfigDefault = "$KUBECONFIG"
	} else if home, err := os.UserHomeDir(); err == nil {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "")
		kubeconfigDefault = ""
	}

	namespace := flag.String("n", "", "")
	flag.StringVar(namespace, "namespace", "", "")
	allNamespaces := flag.Bool("A", false, "")
	watchFlag := flag.Bool("w", false, "")
	flag.BoolVar(watchFlag, "watch", false, "")
	versionFlag := flag.Bool("version", false, "")
	flag.BoolVar(versionFlag, "v", false, "")

	flag.Usage = func() { printUsage(kubeconfigDefault) }
	flag.Parse()

	if *versionFlag {
		fmt.Println("kgps", version)
		return
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		die("loading kubeconfig", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		die("creating Kubernetes client", err)
	}

	targetNamespace := *namespace
	if *allNamespaces {
		targetNamespace = ""
	} else if targetNamespace == "" {
		clientCfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
		if err != nil {
			die("loading kubeconfig rules", err)
		}
		currentContext := clientCfg.Contexts[clientCfg.CurrentContext]
		if currentContext != nil {
			targetNamespace = currentContext.Namespace
		}
		if targetNamespace == "" {
			targetNamespace = "default"
		}
	}

	pods, err := clientset.CoreV1().Pods(targetNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		die("listing pods", err)
	}

	rows := make([]podRow, 0, len(pods.Items))
	for _, pod := range pods.Items {
		rows = append(rows, buildRow(pod))
	}

	widths := printTable(rows, *allNamespaces)

	if !*watchFlag {
		return
	}

	watcher, err := clientset.CoreV1().Pods(targetNamespace).Watch(context.TODO(), metav1.ListOptions{
		ResourceVersion: pods.ResourceVersion,
	})
	if err != nil {
		die("starting watch", err)
	}
	defer watcher.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sig:
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return
			}
			pod, ok := event.Object.(*v1.Pod)
			if !ok {
				continue
			}
			row := buildRow(*pod)
			// Expand column widths if this pod is wider than what we've seen.
			cols := rowCols(row, *allNamespaces)
			for i, c := range cols {
				if len(c) > widths[i] {
					widths[i] = len(c)
				}
			}
			if event.Type == watch.Deleted {
				printRow(row, widths, *allNamespaces, true)
			} else {
				printRow(row, widths, *allNamespaces, false)
			}
		}
	}
}

// printTable renders the full table and returns the computed column widths.
func printTable(rows []podRow, allNamespaces bool) []int {
	const minPad = 2

	headers := tableHeaders(allNamespaces)
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range rowCols(r, allNamespaces) {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	pad := func(s string, w int) string {
		return s + strings.Repeat(" ", w-len(s))
	}

	// header
	var hb strings.Builder
	for i, h := range headers {
		if i < len(headers)-1 {
			hb.WriteString(pad(h, widths[i]+minPad))
		} else {
			hb.WriteString(h)
		}
	}
	color.Style{color.Bold, color.FgWhite}.Println(hb.String())

	for _, r := range rows {
		printRow(r, widths, allNamespaces, false)
	}

	return widths
}

func printRow(r podRow, widths []int, allNamespaces bool, deleted bool) {
	const minPad = 2
	headers := tableHeaders(allNamespaces)
	cols := rowCols(r, allNamespaces)
	runningNotReady := r.status == "Running" && r.readyCount < r.totalCount

	pad := func(s string, w int) string {
		return s + strings.Repeat(" ", w-len(s))
	}

	var lb strings.Builder
	for i, raw := range cols {
		isLast := i == len(cols)-1
		var padded string
		if isLast {
			padded = raw
		} else {
			padded = pad(raw, widths[i]+minPad)
		}

		var segment string
		if deleted {
			segment = color.FgWhite.Sprint(padded)
			lb.WriteString(segment)
			continue
		}

		switch headers[i] {
		case "NAMESPACE":
			segment = color.FgWhite.Sprint(padded)
		case "NAME":
			segment = color.FgWhite.Sprint(padded)
		case "READY":
			if r.status == "Completed" || r.status == "Succeeded" {
				segment = color.FgWhite.Sprint(padded)
			} else if runningNotReady {
				segment = color.FgYellow.Sprint(padded)
			} else if r.readyCount == r.totalCount && r.totalCount > 0 {
				segment = color.FgGreen.Sprint(padded)
			} else {
				segment = color.FgRed.Sprint(padded)
			}
		case "STATUS":
			segment = colorStatus(padded, r.status, runningNotReady)
		case "RESTARTS", "AGE":
			segment = color.FgWhite.Sprint(padded)
		case "LAST RESTART REASON":
			segment = colorLastReason(padded, r.lastReason)
		default:
			segment = padded
		}
		lb.WriteString(segment)
	}
	fmt.Println(lb.String())
}

func tableHeaders(allNamespaces bool) []string {
	if allNamespaces {
		return []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "AGE", "LAST RESTART REASON"}
	}
	return []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE", "LAST RESTART REASON"}
}

func buildRow(pod v1.Pod) podRow {
	readyContainers := 0
	totalContainers := len(pod.Spec.Containers)
	restarts := 0
	lastRestartReason := ""

	for i := range pod.Status.ContainerStatuses {
		restarts += int(pod.Status.ContainerStatuses[i].RestartCount)
		if pod.Status.ContainerStatuses[i].Ready {
			readyContainers++
		}
		if pod.Status.ContainerStatuses[i].LastTerminationState.Terminated != nil {
			if pod.Status.ContainerStatuses[i].LastTerminationState.Terminated.Reason != "" {
				lastRestartReason = pod.Status.ContainerStatuses[i].LastTerminationState.Terminated.Reason
			}
		}
	}

	return podRow{
		namespace:  pod.Namespace,
		name:       pod.Name,
		ready:      fmt.Sprintf("%d/%d", readyContainers, totalContainers),
		readyCount: readyContainers,
		totalCount: totalContainers,
		status:     getPodStatus(pod),
		restarts:   fmt.Sprintf("%d", restarts),
		age:        duration.HumanDuration(time.Since(pod.CreationTimestamp.Time)),
		lastReason: lastRestartReason,
	}
}

func colorStatus(padded, status string, runningNotReady bool) string {
	if runningNotReady {
		return color.FgYellow.Sprint(padded)
	}
	switch status {
	case "Running", "Succeeded", "Completed":
		return color.FgGreen.Sprint(padded)
	case "Pending", "ContainerCreating", "Terminating", "Init:0/1":
		return color.FgYellow.Sprint(padded)
	case "Failed", "Error", "CrashLoopBackOff", "OOMKilled", "ImagePullBackOff", "ErrImagePull":
		return color.FgRed.Sprint(padded)
	default:
		return color.FgWhite.Sprint(padded)
	}
}

func colorLastReason(padded, reason string) string {
	switch reason {
	case "OOMKilled", "Error", "ContainerCannotRun", "DeadlineExceeded":
		return color.FgRed.Sprint(padded)
	case "Completed":
		return color.FgGreen.Sprint(padded)
	case "":
		return color.FgWhite.Sprint(padded)
	default:
		return color.FgYellow.Sprint(padded)
	}
}

func rowCols(r podRow, allNamespaces bool) []string {
	if allNamespaces {
		return []string{r.namespace, r.name, r.ready, r.status, r.restarts, r.age, r.lastReason}
	}
	return []string{r.name, r.ready, r.status, r.restarts, r.age, r.lastReason}
}

func getPodStatus(pod v1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}

	if pod.Status.Reason != "" {
		return pod.Status.Reason
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
			return cs.State.Terminated.Reason
		}
	}

	return string(pod.Status.Phase)
}
