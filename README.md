# kgps — Kubernetes Get Pod Status

Like `kubectl get pods`, only with last restart reason.

```
NAME                          READY   STATUS    RESTARTS   AGE    LAST RESTART REASON
my-app-7d9f8b6c5-xk2pq        2/2     Running   0          3d2h
redis-0                        1/1     Running   4          7d     OOMKilled
broken-job-59f7d-hzmnp         0/1     Error     12         1h     Error
```

## Features

- Color-coded status: green (healthy), yellow (pending/not-ready), red (failed)
- Watch mode for real-time updates (`-w`)
- All-namespaces view (`-A`)
- Filter to only pods with restarts (`-r`)
- Shows last restart reason per container
- Adapts column widths dynamically

## Installation

```sh
go install github.com/jessegoodier/kgps@latest
```

Or build from source:

```sh
git clone https://github.com/jessegoodier/kgps.git
cd kgps
go build -o kgps .
```

## Usage

```
kgps [flags]

Flags:
  -n, --namespace <namespace>  Namespace to list pods in (default: current context's namespace)
  -A                          List pods across all namespaces
  -w, --watch                  Watch for pod changes
  -r, --has-restarts           Only show pods with restarts
  -kubeconfig <path>          Path to kubeconfig file (default: $KUBECONFIG, then ~/.kube/config)
  -v, --version                Print version and exit
  -h, --help                   Show help
```

### Examples

```sh
# List pods in current namespace
kgps

# List pods in a specific namespace
kgps -n production

# List pods across all namespaces
kgps -A

# Only show pods that have restarted
kgps -r
kgps -A -r

# Watch for changes in real-time
kgps -w
kgps -n staging -w

# Use a custom kubeconfig
kgps -kubeconfig /path/to/kubeconfig
```

## Requirements

- Go 1.21+
- A valid kubeconfig with cluster access

## License

MIT
