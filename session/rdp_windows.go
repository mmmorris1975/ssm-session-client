//go:build windows

package session

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// StartRDPSession starts an RDP session to the target EC2 instance via SSM port forwarding.
// It sets up the SSM tunnel, optionally retrieves the EC2 admin password, then launches mstsc.exe.
func StartRDPSession(target string) error {
	_, host, _, err := ParseHostPort(target, "", 3389)
	if err != nil {
		zap.S().Fatal(err)
	}

	ssmcfg, err := BuildAWSConfig(context.Background(), "ssm")
	if err != nil {
		zap.S().Fatal(err)
	}

	tgt, err := ssmclient.ResolveTarget(host, ssmcfg)
	if err != nil {
		zap.S().Fatal(err)
	}

	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}

	rdpPort := config.Flags().RDPPort
	if rdpPort == 0 {
		rdpPort = 3389
	}

	localPort, err := allocateFreePort()
	if err != nil {
		return fmt.Errorf("allocating local port: %w", err)
	}
	if config.Flags().RDPLocalPort != 0 {
		localPort = config.Flags().RDPLocalPort
	}

	username := config.Flags().RDPUsername
	if username == "" {
		username = "Administrator"
	}

	var password string
	if config.Flags().RDPGetPassword {
		ec2cfg, err := BuildAWSConfig(context.Background(), "ec2")
		if err != nil {
			zap.S().Fatal(err)
		}
		password, err = GetEC2Password(context.Background(), ec2cfg, tgt, config.Flags().RDPKeyPairFile)
		if err != nil {
			return fmt.Errorf("retrieving EC2 password: %w", err)
		}
		zap.S().Info("Successfully retrieved EC2 administrator password")

		if err := setClipboard(password); err != nil {
			zap.S().Warnf("could not copy password to clipboard: %v", err)
		} else {
			fmt.Fprintln(os.Stderr, "Password copied to clipboard (will be cleared after mstsc exits)")
			defer clearClipboard()
		}
	}
	// When --get-password is not set, let mstsc.exe prompt for credentials
	// natively. Embedding passwords in the .rdp file is blocked by the
	// "Do not allow passwords to be saved" Group Policy on most servers.

	readyCh := make(chan struct{})
	pfInput := &ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: rdpPort,
		LocalPort:  localPort,
		ReadyCh:    readyCh,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pfErrCh := make(chan error, 1)
	go func() {
		pfErrCh <- ssmclient.PortForwardingSessionWithContext(ctx, ssmMessagesCfg, pfInput)
	}()

	// Wait for the TCP listener to be ready before launching mstsc.exe
	select {
	case <-readyCh:
		// Listener is up
	case err := <-pfErrCh:
		cancel()
		return fmt.Errorf("SSM port forwarding failed: %w", err)
	case <-time.After(60 * time.Second):
		cancel()
		return fmt.Errorf("timed out waiting for SSM port forwarding to become ready")
	}

	rdpOpts := &ssmclient.RDPInput{
		LocalPort: localPort,
		Username:  username,
		Password:  password,
	}

	rdpErr := ssmclient.RDPSession(ctx, rdpOpts)

	// Cancel port forwarding after mstsc.exe exits
	cancel()

	// Give the port forwarder a moment to shut down cleanly
	select {
	case <-pfErrCh:
	case <-time.After(3 * time.Second):
	}

	return rdpErr
}

// allocateFreePort finds a free TCP port on localhost by briefly binding to :0.
func allocateFreePort() (int, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// setClipboard copies text to the Windows clipboard using the native Win32 API.
func setClipboard(text string) error {
	r, _, err := procOpenClipboard.Call(0)
	if r == 0 {
		return fmt.Errorf("OpenClipboard: %w", err)
	}
	defer procCloseClipboard.Call() //nolint:errcheck

	procEmptyClipboard.Call() //nolint:errcheck

	// Convert to UTF-16 with null terminator
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := len(utf16) * 2

	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(size))
	if h == 0 {
		return fmt.Errorf("GlobalAlloc: %w", err)
	}

	ptr, _, err := procGlobalLock.Call(h)
	if ptr == 0 {
		return fmt.Errorf("GlobalLock: %w", err)
	}

	src := unsafe.Slice((*byte)(unsafe.Pointer(&utf16[0])), size)
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size) //nolint:unsafeptr // ptr is from GlobalLock
	copy(dst, src)

	procGlobalUnlock.Call(h) //nolint:errcheck

	r, _, err = procSetClipboardData.Call(cfUnicodeText, h)
	if r == 0 {
		return fmt.Errorf("SetClipboardData: %w", err)
	}

	return nil
}

// clearClipboard empties the Windows clipboard.
func clearClipboard() {
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return
	}
	procEmptyClipboard.Call() //nolint:errcheck
	procCloseClipboard.Call() //nolint:errcheck
}

