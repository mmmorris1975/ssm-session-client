# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`ssm-session-client` is a Go CLI tool providing AWS SSM Session Manager access in a single self-contained binary. It supports shell sessions, SSH proxy/direct, EC2 Instance Connect, port forwarding, and RDP — useful in restricted environments (AppLocker, AirLock) and complex VPN/Direct Connect networks where AWS PrivateLink endpoints are accessible only from private networks.

## Commands

```bash
# Build for current platform
go build -o ssm-session-client main.go

# Cross-compile (examples)
GOOS=linux   GOARCH=amd64 go build -o ssm-session-client-linux main.go
GOOS=darwin  GOARCH=arm64 go build -o ssm-session-client-darwin-arm64 main.go
GOOS=windows GOARCH=amd64 go build -o ssm-session-client.exe main.go

# Test (with race detector and coverage)
go test ./... -race -coverprofile=coverage.out -covermode=atomic
go tool cover -func=coverage.out

# Run a single test
go test ./ssmclient/... -run TestTargetResolver -race

# Lint
golangci-lint run
```

## Architecture

### Package Responsibilities

- **`main.go`** — Detects OpenSSH-compat mode (for VSCode Remote SSH) and routes to either the SSH compat handler or the Cobra CLI dispatcher.
- **`cmd/`** — Cobra command definitions. Each command parses flags, loads config via Viper, then delegates to `pkg/`.
- **`config/`** — Config struct (35+ fields), Viper binding (flags → env vars → YAML file), and zap logger setup with lumberjack rotation.
- **`session/`** — Business logic: AWS SDK initialization (`client.go`), SSO device-code login (`sso.go`), OpenSSH arg parsing for compat mode, EC2 Instance Connect ephemeral key management, and per-command orchestration (`ssm_shell.go`, `ssm_ssh_direct.go`, etc.).
- **`ssmclient/`** — Core SSM session implementations:
  - `target_resolver.go`: Resolves targets via 5 strategies: direct instance ID, alias, tag lookup, IP address, and DNS TXT record.
  - `ssm_conn.go`: Bridges the SSM data channel to a `net.Conn` using `net.Pipe()` for SSH transport.
  - `ssh_direct.go`: Native Go SSH (TOFU host key, agent/key/password/ephemeral auth).
  - `port_forwarding.go` + `port_mux.go`: Port forwarding with stream multiplexing for SSM agent ≥ 3.0.196.0.
  - Platform-specific terminal handling in `shell_posix.go`, `shell_linux.go`, `shell_bsdish.go`, `shell_windows.go`.
- **`datachannel/`** — WebSocket data channel:
  - `data_channel.go`: WebSocket lifecycle, handshake, message queuing, reconnection.
  - `agent_message.go`: Binary AgentMessage protocol (serialization, SHA256 digest, schema versioning).
  - `encryption.go`: Optional AES-256-GCM encryption using AWS KMS `GenerateDataKey`, with sessionId+targetId as encryption context.

### Key Data Flows

**Shell:** `cmd/ssm_shell.go` → `session/ssm_shell.go` → `ssmclient/target_resolver.go` → `ssmclient/shell.go` → `datachannel/data_channel.go` (WebSocket to AWS SSM StartSession API)

**SSH-Direct:** same start → `ssmclient/ssh_direct.go` → `ssmclient/ssm_conn.go` (net.Pipe bridge) → `golang.org/x/crypto/ssh` client

**Port Forwarding:** → `ssmclient/port_forwarding.go` → local TCP listener → `ssmclient/port_mux.go` (smux multiplexer for modern agents) or direct copy for older agents

### Configuration Priority

1. CLI flags
2. Environment variables (`SSC_` prefix)
3. YAML config file (`.ssm-session-client.yaml`)

VPC endpoint overrides are supported for STS, SSM, SSM Messages, EC2, and KMS to support PrivateLink-only environments.

### Linting Constraints

- Max function length: 75 lines / 50 statements
- Max cognitive/cyclomatic complexity: 15
- Max line length: 132 characters
- Key enabled linters: `errcheck`, `gosec`, `funlen`, `gocognit`, `gocyclo`, `revive`, `bodyclose`

## Documentation Website

A static documentation site lives in `docs/` and is published to GitHub Pages.

### Structure

```
docs/
  index.html       # Single-page documentation site
  css/style.css    # Stylesheet (AWS-inspired colour scheme, responsive)
  js/main.js       # Tabs, copy buttons, mobile menu, scroll spy
```

### Sections

Overview · Install (macOS/Linux/Windows tabs) · Configuration · Session Modes · Troubleshooting · Contributing

### Local Preview

```bash
cd docs && python3 -m http.server 8080
# then open http://localhost:8080
```

### Deployment

`.github/workflows/pages.yml` deploys automatically when a release is published (`release: types: [published]`). It also redeploys on pushes to `main` that touch `docs/**`.

**Version injection:** The workflow replaces the `%%VERSION%%` placeholder in `index.html` with the release tag (e.g. `v1.2.3`) via `sed` before uploading the Pages artifact. For local preview or doc-only pushes where no tag is available, `js/main.js` fetches the latest release tag from the GitHub API at runtime.

**GitHub Pages setup (one-time):** Repo Settings → Pages → Source: **GitHub Actions**.
