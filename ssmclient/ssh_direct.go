package ssmclient

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/datachannel"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// SSHDirectInput configures an SSH direct session.
type SSHDirectInput struct {
	Target             string     // EC2 instance ID
	User               string     // SSH username
	RemotePort         int        // SSH port (default 22)
	KeyFile            string     // Path to private key file (optional, empty = auto-discover)
	NoHostKeyCheck     bool       // Skip host key verification
	ExecCommand        string     // Command to execute; empty means interactive shell
	EphemeralSigner    ssh.Signer // In-memory signer (e.g. from EC2 Instance Connect ephemeral key)
	DisablePTY         bool       // When true, do not allocate a PTY (equivalent to ssh -T)
	KnownHostsFile     string     // Custom known_hosts file path (empty = ~/.ssh/known_hosts)
	ConnectTimeoutSecs int        // Connection timeout in seconds (0 = no timeout)
	DynamicForward     string     // SOCKS5 dynamic port forwarding bind address (-D flag)
}

// SSHDirectSession establishes a direct SSH connection to an EC2 instance via SSM
// without requiring an external SSH client. It opens an AWS-StartSSHSession tunnel,
// then uses golang.org/x/crypto/ssh for the SSH layer.
func SSHDirectSession(cfg aws.Config, opts *SSHDirectInput) error {
	port := "22"
	if opts.RemotePort > 0 {
		port = strconv.Itoa(opts.RemotePort)
	}

	in := &ssm.StartSessionInput{
		DocumentName: aws.String("AWS-StartSSHSession"),
		Target:       aws.String(opts.Target),
		Parameters: map[string][]string{
			"portNumber": {port},
		},
	}

	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, in, &datachannel.SSMMessagesResover{
		Endpoint: config.Flags().SSMMessagesVpcEndpoint,
	}); err != nil {
		return err
	}
	defer func() {
		_ = c.TerminateSession()
		_ = c.Close()
	}()

	installSignalHandler(c)

	zap.S().Info("waiting for SSM handshake")
	if err := c.WaitForHandshakeComplete(context.Background()); err != nil {
		return fmt.Errorf("SSM handshake failed: %w", err)
	}
	zap.S().Info("SSM handshake complete, establishing SSH connection")

	ssmConn := NewSSMConn(c)

	hostKeyCallback, err := buildHostKeyCallback(opts.Target, opts.NoHostKeyCheck, opts.KnownHostsFile)
	if err != nil {
		return fmt.Errorf("host key setup failed: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            opts.User,
		Auth:            buildSSHAuthMethods(opts.KeyFile, opts.EphemeralSigner),
		HostKeyCallback: hostKeyCallback,
	}

	// ssh.NewClientConn requires host:port so knownhosts can normalize the address.
	// net.Pipe() returns "pipe" for RemoteAddr() which causes knownhosts to fail
	// with SplitHostPort. Wrap the conn so RemoteAddr() returns the real target address.
	sshAddr := net.JoinHostPort(opts.Target, port)
	sshConn, chans, reqs, err := ssh.NewClientConn(&ssmAddrConn{Conn: ssmConn, addr: ssmAddr(sshAddr)}, sshAddr, sshConfig)
	if err != nil {
		printSSHAuthDiagnostic(err, opts)
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	zap.S().Info("SSH connection established")

	// Start SOCKS5 dynamic port forwarding if -D was specified.
	if opts.DynamicForward != "" {
		ln, err := startSOCKS5Proxy(client, opts.DynamicForward)
		if err != nil {
			return fmt.Errorf("SOCKS5 proxy failed: %w", err)
		}
		defer ln.Close()
		zap.S().Infof("SOCKS5 proxy listening on %s", ln.Addr())
	}

	if opts.ExecCommand != "" {
		return runSSHCommand(client, opts.ExecCommand)
	}
	if opts.DisablePTY {
		return runNoPTYSession(client)
	}
	return runInteractiveSSHSession(client)
}

// buildSSHAuthMethods constructs the authentication method chain.
// When an ephemeral signer is provided (e.g. from EC2 Instance Connect) it is
// tried first, before SSH agent and key-file methods.
// Order: ephemeral key → SSH agent → private key file → password prompt.
func buildSSHAuthMethods(keyFile string, ephemeral ssh.Signer) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	var methodNames []string

	if ephemeral != nil {
		methods = append(methods, ssh.PublicKeys(ephemeral))
		methodNames = append(methodNames, "instance-connect-ephemeral-key")
	}

	if method := trySSHAgentAuth(); method != nil {
		methods = append(methods, method)
		methodNames = append(methodNames, "ssh-agent")
	}

	if signer, err := loadSSHPrivateKey(keyFile); err == nil {
		methods = append(methods, ssh.PublicKeys(signer))
		methodNames = append(methodNames, "private-key-file")
	} else if keyFile != "" {
		zap.S().Warnf("failed to load SSH private key %q: %v", keyFile, err)
	}

	methods = append(methods, ssh.PasswordCallback(promptPassword))
	methodNames = append(methodNames, "password-prompt")

	zap.S().Infof("SSH auth methods: %v", methodNames)
	return methods
}

// printSSHAuthDiagnostic prints a human-readable diagnostic to stderr when SSH
// authentication fails, helping users identify infrastructure issues without
// needing to dig through log files.
func printSSHAuthDiagnostic(err error, opts *SSHDirectInput) {
	if !strings.Contains(err.Error(), "unable to authenticate") {
		return
	}
	fmt.Fprintln(os.Stderr, "\n--- SSH Authentication Failed ---")
	fmt.Fprintf(os.Stderr, "Target: %s | User: %s\n", opts.Target, opts.User)
	if opts.EphemeralSigner != nil {
		fmt.Fprintln(os.Stderr, "  [x] EC2 Instance Connect ephemeral key was provided")
		fmt.Fprintln(os.Stderr, "      Check: Is the EC2 Instance Connect agent installed and running on the target?")
		fmt.Fprintln(os.Stderr, "      Check: Does the OS user '"+opts.User+"' exist on the instance?")
	}
	if opts.KeyFile != "" {
		fmt.Fprintf(os.Stderr, "  [x] Private key file: %s\n", opts.KeyFile)
	}
	fmt.Fprintln(os.Stderr, "Enable debug logging (--log-level debug) for full details.")
	fmt.Fprintln(os.Stderr, "---------------------------------")
}

// trySSHAgentAuth returns an SSH auth method backed by the running SSH agent, or
// nil if SSH_AUTH_SOCK is not set or the agent cannot be reached.
func trySSHAgentAuth() ssh.AuthMethod {
	sockPath := os.Getenv("SSH_AUTH_SOCK")
	if sockPath == "" {
		return nil
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		zap.S().Debugf("SSH agent connect failed: %v", err)
		return nil
	}

	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers)
}

// loadSSHPrivateKey loads an SSH private key from the given path, or
// auto-discovers one via config.FindSSHPrivateKey if path is empty.
func loadSSHPrivateKey(keyFile string) (ssh.Signer, error) {
	path, err := config.FindSSHPrivateKey(keyFile)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return parsePrivateKeyWithPrompt(data, path)
	}

	zap.S().Infof("using SSH key: %s", path)
	return signer, nil
}

// parsePrivateKeyWithPrompt tries to parse an encrypted private key by
// interactively prompting for its passphrase.
func parsePrivateKeyWithPrompt(data []byte, path string) (ssh.Signer, error) {
	fmt.Fprintf(os.Stderr, "Enter passphrase for %s: ", path)
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKeyWithPassphrase(data, passphrase)
}

// promptPassword interactively prompts for a password.
func promptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(pw), err
}

// buildHostKeyCallback returns a host key verification callback. When noCheck is
// true the callback accepts any key (with a warning). Otherwise it checks
// the known_hosts file and falls back to a Trust-On-First-Use prompt.
// If customKnownHosts is non-empty, it is used instead of ~/.ssh/known_hosts.
func buildHostKeyCallback(_ string, noCheck bool, customKnownHosts string) (ssh.HostKeyCallback, error) {
	if noCheck {
		zap.S().Warn("host key verification disabled (--no-host-key-check)")
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec
	}

	var knownHostsFile string
	if customKnownHosts != "" {
		knownHostsFile = customKnownHosts
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		knownHostsFile = filepath.Join(homeDir, ".ssh", "known_hosts")
	}
	if _, err := os.Stat(knownHostsFile); err == nil {
		knownHostsCb, err := knownhosts.New(knownHostsFile)
		if err != nil {
			zap.S().Warnf("failed to parse known_hosts: %v", err)
		} else {
			return tofuHostKeyCallback(knownHostsCb, knownHostsFile), nil
		}
	}

	return tofuHostKeyCallback(nil, knownHostsFile), nil
}

// tofuHostKeyCallback wraps an optional known_hosts callback with Trust-On-First-Use
// behaviour: unknown hosts receive an interactive prompt and accepted keys are
// appended to knownHostsFile.
func tofuHostKeyCallback(knownHostsCb ssh.HostKeyCallback, knownHostsFile string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if knownHostsCb != nil {
			err := knownHostsCb(hostname, remote, key)
			if err == nil {
				return nil
			}
			// A non-empty Want list means the key changed — reject immediately.
			var keyErr *knownhosts.KeyError
			if !errors.As(err, &keyErr) || len(keyErr.Want) > 0 {
				return err
			}
		}

		fp := ssh.FingerprintSHA256(key)
		fmt.Fprintf(os.Stderr, "The authenticity of host '%s' can't be established.\n", hostname)
		fmt.Fprintf(os.Stderr, "%s key fingerprint is %s.\n", key.Type(), fp)
		fmt.Fprint(os.Stderr, "Are you sure you want to continue connecting (yes/no)? ")

		var answer string
		if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil || (answer != "yes" && answer != "y") {
			return fmt.Errorf("host key verification failed: user declined")
		}

		if err := appendKnownHost(knownHostsFile, hostname, key); err != nil {
			zap.S().Warnf("failed to save host key: %v", err)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Permanently added '%s' to the list of known hosts.\n", hostname)
		}

		return nil
	}
}

// ssmAddr is a net.Addr whose String() returns a "host:port" value, satisfying
// golang.org/x/crypto/ssh/knownhosts which calls net.SplitHostPort on it.
type ssmAddr string

func (a ssmAddr) Network() string { return "tcp" }
func (a ssmAddr) String() string  { return string(a) }

// ssmAddrConn wraps a net.Conn to override RemoteAddr() with a real host:port.
// net.Pipe() connections return "pipe" which breaks knownhosts.SplitHostPort.
type ssmAddrConn struct {
	net.Conn
	addr ssmAddr
}

func (c *ssmAddrConn) RemoteAddr() net.Addr { return c.addr }

// appendKnownHost appends a single host key line to the known_hosts file,
// creating the file if it does not exist.
func appendKnownHost(knownHostsFile, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, knownhosts.Line([]string{hostname}, key))
	return err
}

// runInteractiveSSHSession requests a PTY and starts an interactive shell over
// the established SSH client connection.
func runInteractiveSSHSession(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	rows, cols, err := getWinSize()
	if err != nil {
		rows, cols = 45, 132
	}

	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty(termType, int(rows), int(cols), modes); err != nil {
		return fmt.Errorf("request PTY: %w", err)
	}

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := configureStdin(); err != nil {
		zap.S().Warnf("failed to set raw terminal: %v", err)
	}
	defer cleanup() //nolint:errcheck

	handleSSHWindowResize(session)

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}

	if err := session.Wait(); err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			// Propagate the remote exit code directly (e.g. 130 for Ctrl+C).
			// This avoids a spurious FATAL log for normal interactive session exits.
			os.Exit(exitErr.ExitStatus())
		}
		return err
	}
	return nil
}

// runNoPTYSession starts an SSH session without PTY allocation. Stdin, stdout,
// and stderr are connected directly. This is used when -T is passed (e.g. by
// VSCode Remote SSH) where the client communicates over raw stdin/stdout.
func runNoPTYSession(client *ssh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}

	if err := session.Wait(); err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitStatus())
		}
		return err
	}
	return nil
}

// runSSHCommand executes a single command over the SSH connection, streaming
// stdout/stderr, and exits with the remote command's exit code on failure.
func runSSHCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session: %w", err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Run(command); err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitStatus())
		}
		return err
	}

	return nil
}

// startSOCKS5Proxy starts a minimal SOCKS5 proxy (no-auth, CONNECT only) that
// forwards connections through the SSH tunnel. bindAddr is either a port number
// ("1080") or a host:port ("127.0.0.1:1080").
func startSOCKS5Proxy(client *ssh.Client, bindAddr string) (net.Listener, error) {
	// Normalize bind address: bare port → 127.0.0.1:port
	if _, err := strconv.Atoi(bindAddr); err == nil {
		bindAddr = "127.0.0.1:" + bindAddr
	}

	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", bindAddr, err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handleSOCKS5Conn(client, conn)
		}
	}()

	return ln, nil
}

// handleSOCKS5Conn handles a single SOCKS5 client connection.
func handleSOCKS5Conn(client *ssh.Client, conn net.Conn) {
	defer conn.Close()

	targetAddr, err := socks5Handshake(conn)
	if err != nil {
		zap.S().Debugf("SOCKS5 handshake failed: %v", err)
		return
	}

	remote, err := client.Dial("tcp", targetAddr)
	if err != nil {
		zap.S().Debugf("SOCKS5 dial %s failed: %v", targetAddr, err)
		// Send connection refused reply
		_ = socks5SendReply(conn, 0x05) // connection refused
		return
	}
	defer remote.Close()

	// Send success reply
	if err := socks5SendReply(conn, 0x00); err != nil {
		return
	}

	// Bidirectional relay
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(remote, conn)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, remote)
	}()
	wg.Wait()
}

// socks5Handshake performs the SOCKS5 version negotiation and CONNECT request,
// returning the target address as "host:port".
func socks5Handshake(conn net.Conn) (string, error) {
	// --- Version/method negotiation ---
	// Client sends: VER | NMETHODS | METHODS...
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", fmt.Errorf("read version header: %w", err)
	}
	if header[0] != 0x05 {
		return "", fmt.Errorf("unsupported SOCKS version: %d", header[0])
	}
	nMethods := int(header[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", fmt.Errorf("read methods: %w", err)
	}

	// Reply: no authentication required (0x00)
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return "", fmt.Errorf("write auth reply: %w", err)
	}

	// --- CONNECT request ---
	// Client sends: VER | CMD | RSV | ATYP | DST.ADDR | DST.PORT
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, reqHeader); err != nil {
		return "", fmt.Errorf("read request header: %w", err)
	}
	if reqHeader[0] != 0x05 {
		return "", fmt.Errorf("unexpected version in request: %d", reqHeader[0])
	}
	if reqHeader[1] != 0x01 { // CONNECT
		return "", fmt.Errorf("unsupported SOCKS5 command: %d", reqHeader[1])
	}

	// Parse destination address
	var host string
	switch reqHeader[3] { // ATYP
	case 0x01: // IPv4
		addr := make([]byte, 4)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv4 addr: %w", err)
		}
		host = net.IP(addr).String()
	case 0x03: // Domain name
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", fmt.Errorf("read domain length: %w", err)
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", fmt.Errorf("read domain: %w", err)
		}
		host = string(domain)
	case 0x04: // IPv6
		addr := make([]byte, 16)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read IPv6 addr: %w", err)
		}
		host = net.IP(addr).String()
	default:
		return "", fmt.Errorf("unsupported address type: %d", reqHeader[3])
	}

	// Read port (2 bytes, big-endian)
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", fmt.Errorf("read port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)

	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// socks5SendReply sends a SOCKS5 reply with the given status code.
// Uses 0.0.0.0:0 as the bind address (not meaningful for CONNECT).
func socks5SendReply(conn net.Conn, status byte) error {
	// VER | REP | RSV | ATYP | BND.ADDR (4 bytes IPv4) | BND.PORT (2 bytes)
	reply := []byte{0x05, status, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	_, err := conn.Write(reply)
	return err
}

// handleSSHWindowResize polls the local terminal size every ResizeSleepInterval
// and sends a WindowChange request to the SSH session when it changes.
func handleSSHWindowResize(session *ssh.Session) {
	var lastRows, lastCols uint32
	go func() {
		for {
			rows, cols, err := getWinSize()
			if err != nil {
				rows, cols = 45, 132
			}
			if rows != lastRows || cols != lastCols {
				_ = session.WindowChange(int(rows), int(cols))
				lastRows, lastCols = rows, cols
			}
			time.Sleep(ResizeSleepInterval)
		}
	}()
}
