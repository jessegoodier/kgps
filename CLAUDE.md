# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
# Build
go build -o kgps .

# Build with version injected (mirrors what the release workflow does)
go build -ldflags "-s -w -X main.version=x.y.z" -o kgps .

# Run directly
go run . [flags]

# Check for vulnerabilities
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

There are no tests yet.

## Architecture

This is a single-file Go CLI (`main.go`) with no subpackages. Everything lives in `package main`.

**Data flow:**
1. Parse flags → resolve target namespace from flag / kubeconfig context
2. List pods via `client-go` → convert each `v1.Pod` to a `podRow` struct (`buildRow`); skip zero-restart pods when `-r` is set
3. Render table with dynamic column widths (`printTable` / `printRow`)
4. Optionally enter a watch loop that receives `watch.Event`s and prints individual rows as they arrive (same `-r` filter applies)

**Key types and functions:**
- `podRow` — flat struct holding all display fields including `readyCount`/`totalCount` (used for color logic, not just the formatted `ready` string)
- `buildRow(pod v1.Pod) podRow` — extracts restart count, ready containers, last termination reason, and calls `getPodStatus`
- `getPodStatus` — prefers waiting/terminated reason over raw phase; returns `"Terminating"` when `DeletionTimestamp` is set
- `printTable` / `printRow` — column-aware rendering; `printRow` takes a `deleted bool` to grey out removed pods in watch mode
- `colorStatus` / `colorLastReason` — map status strings to `gookit/color` styles
- `printUsage` — custom `flag.Usage` replacement with colored output; takes `kubeconfigDefault` string so the help line reflects the active default (`$KUBECONFIG` env var → `~/.kube/config` → empty)

**Version injection:** `var version = "dev"` at package level, overridden at link time with `-X main.version=...`. The `VERSION` file in the repo root is the source of truth read by the release workflow — it is not read at runtime.

## Release workflow

Triggered manually via `workflow_dispatch` with a `bump` input (patch/minor/major). Jobs run in sequence: `security` → `version` → `build` (6-target matrix) → `release`. The `version` job commits the bumped `VERSION` file and pushes a tag; build jobs check out at that tag. See `.github/workflows/release.yml`.
