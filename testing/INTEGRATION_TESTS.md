# Integration Test Matrix

This document describes the manual integration test matrix for ssm-session-client. These tests verify cross-platform compatibility and real-world SSM session functionality.

## Test Environment Requirements

- AWS account with EC2 instances configured for SSM
- SSM agent installed and running on target instances
- Appropriate IAM permissions for SSM sessions
- Local AWS credentials configured (via `aws configure` or environment variables)

## Test Matrix

### Shell Sessions

| Client Platform | Target Platform | Test Command | Expected Result |
|----------------|-----------------|--------------|-----------------|
| Windows | Linux EC2 | `ssm-session-client shell -t <instance-id>` | Interactive shell with proper terminal control (raw mode, VT sequences) |
| Windows | Windows EC2 | `ssm-session-client shell -t <instance-id>` | Interactive PowerShell/cmd session with proper console behavior |
| Linux/macOS | Windows EC2 | `ssm-session-client shell -t <instance-id>` | Interactive PowerShell/cmd session |
| Linux/macOS | Linux EC2 | `ssm-session-client shell -t <instance-id>` | Interactive bash/sh session |

**Shell Session Test Checklist:**
- [ ] Terminal resize (change window size, verify `$COLUMNS` and `$LINES` update)
- [ ] Raw input mode (arrow keys, Ctrl+C, Ctrl+D work correctly)
- [ ] VT escape sequences (colors, cursor positioning)
- [ ] Special characters and Unicode
- [ ] Clean exit (Ctrl+D or `exit` command)
- [ ] Signal handling (Ctrl+C interrupts running command)

### SSH Sessions (via SSM tunnel)

| Client Platform | Target Platform | Test Command | Expected Result |
|----------------|-----------------|--------------|-----------------|
| Windows | Linux EC2 | `ssm-session-client ssh -t <instance-id>` | SSH connection established through SSM tunnel |
| Linux/macOS | Linux EC2 | `ssm-session-client ssh -t <instance-id>` | SSH connection established through SSM tunnel |

**SSH Session Test Checklist:**
- [ ] Connection establishes without errors
- [ ] Authentication works (key-based or password)
- [ ] Interactive commands work (vim, top, etc.)
- [ ] File transfer (scp/sftp through tunnel if supported)
- [ ] Port forwarding (-L, -R flags if supported)

### Port Forwarding Sessions

| Client Platform | Target Platform | Mode | Test Command | Expected Result |
|----------------|-----------------|------|--------------|-----------------|
| Windows | Linux EC2 | Basic | `ssm-session-client forward -t <instance-id> -r 80 -l 8080` (old agent) | Single connection forwarding to remote port 80 |
| Windows | Linux EC2 | Mux | `ssm-session-client forward -t <instance-id> -r 80 -l 8080` (agent >= 3.0.196) | Multiple concurrent connections to remote port 80 |
| Linux/macOS | Linux EC2 | Basic | `ssm-session-client forward -t <instance-id> -r 3306 -l 3306` (old agent) | Single connection to MySQL |
| Linux/macOS | Linux EC2 | Mux | `ssm-session-client forward -t <instance-id> -r 3306 -l 3306` (agent >= 3.0.196) | Multiple concurrent connections to MySQL |
| Windows | Windows EC2 | Mux | `ssm-session-client forward -t <instance-id> -r 3389 -l 3389` | RDP port forwarding |

**Port Forwarding Test Checklist:**
- [ ] Single connection works correctly (basic and mux modes)
- [ ] Multiple concurrent connections (mux mode only):
  - [ ] Run `curl -s http://localhost:8080` in parallel (e.g., 5 simultaneous requests)
  - [ ] Open multiple database connections (e.g., multiple mysql clients)
  - [ ] Verify all connections succeed without blocking
- [ ] Data integrity (large file transfer, binary data)
- [ ] Connection cleanup (graceful close on both sides)
- [ ] Error handling (connection refused, timeout)
- [ ] Remote host forwarding (`--host` parameter):
  - [ ] `ssm-session-client forward -t <bastion-id> --host <private-host> -r 22 -l 2222`

### Agent Version Detection

Test that the client correctly detects agent version and selects the appropriate port forwarding mode:

| Target Agent Version | Expected Mode | Verification |
|---------------------|---------------|--------------|
| < 3.0.196.0 | Basic (single connection) | Check logs for "using basic port forwarding" |
| >= 3.0.196.0 | Mux (multiple connections) | Check logs for "using multiplexed port forwarding" |
| >= 3.1.1511.0 | Mux with KeepAlive disabled | Verify smux config in code |

**Agent Version Test Commands:**
```bash
# On target instance, check SSM agent version:
sudo systemctl status amazon-ssm-agent
# or
/usr/bin/amazon-ssm-agent --version
```

## Testing Notes

### Windows-Specific Tests

When testing from Windows clients:
1. Use PowerShell or Windows Terminal (not cmd.exe) for best VT support
2. Verify console mode restoration (original settings restored on exit)
3. Test interruption (Ctrl+Break in addition to Ctrl+C)
4. Verify cleanup on abnormal exit (kill process, network disconnect)

### Platform-Specific Build Verification

Before running integration tests, verify local builds work:

```bash
# Windows
go build -o ssm-session-client.exe .
.\ssm-session-client.exe --version

# Linux/macOS
go build -o ssm-session-client .
./ssm-session-client --version
```

### Cross-Compilation Verification

Verify cross-compilation works for all platforms:

```bash
# From any platform:
GOOS=windows GOARCH=amd64 go build -o dist/ssm-session-client-windows.exe .
GOOS=darwin GOARCH=amd64 go build -o dist/ssm-session-client-darwin-amd64 .
GOOS=darwin GOARCH=arm64 go build -o dist/ssm-session-client-darwin-arm64 .
GOOS=linux GOARCH=amd64 go build -o dist/ssm-session-client-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o dist/ssm-session-client-linux-arm64 .
```

## Automated Test Coverage

The following are covered by automated unit and integration tests in CI:

- ✅ Windows console mode configuration and cleanup
- ✅ Version comparison logic
- ✅ Smux session creation and multiplexing
- ✅ Basic port forwarding logic
- ✅ Cross-platform compilation

The following require manual testing with real AWS infrastructure:

- ⚠️ End-to-end SSM session establishment
- ⚠️ Real agent version detection
- ⚠️ Actual network data transfer
- ⚠️ Windows console behavior with real terminal emulators

## Reporting Issues

When reporting platform-specific issues, include:

1. Client platform (OS, version, terminal emulator)
2. Target platform (EC2 instance OS, SSM agent version)
3. Session type (shell, ssh, port forwarding)
4. Expected vs actual behavior
5. Relevant logs (use `-v` or `--debug` flags if available)
6. Minimal reproduction steps
