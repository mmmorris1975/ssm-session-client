package ssmclient

import (
	"context"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/datachannel"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"go.uber.org/zap"
	"golang.org/x/net/netutil"
)

// PortForwardingInput configures the port forwarding session parameters.
// Target is the EC2 instance ID to establish the session with.
// RemotePort is the port on the EC2 instance to connect to.
// LocalPort is the port on the local host to listen to.  If not provided, a random port will be used.
type PortForwardingInput struct {
	Target     string
	RemotePort int
	LocalPort  int
	Host       string          // optional
	ReadyCh    chan struct{}    // optional; closed when the TCP listener is ready
}

// PortForwardingSession starts a port forwarding session using the PortForwardingInput parameters to
// configure the session.  The aws.Config parameter will be used to call the AWS SSM StartSession
// API, which is used as part of establishing the websocket communication channel.
//
// The context's cancellation function is used to close the port forwarding session.
func PortForwardingSessionWithContext(ctx context.Context, cfg aws.Config, opts *PortForwardingInput) error {
	c, err := openDataChannel(cfg, opts)
	if err != nil {
		return err
	}
	defer func() {
		// Both the basic and muxing plugins support TerminateSession on the agent side.
		_ = c.TerminateSession()
		_ = c.Close()
	}()

	return startPortForwardingSession(ctx, c, opts)
}

// PortForwardingSession starts a port forwarding session using the PortForwardingInput parameters to
// configure the session.  The aws.Config parameter will be used to call the AWS SSM StartSession
// API, which is used as part of establishing the websocket communication channel.
func PortForwardingSession(cfg aws.Config, opts *PortForwardingInput) error {
	c, err := openDataChannel(cfg, opts)
	if err != nil {
		return err
	}
	defer func() {
		// Both the basic and muxing plugins support TerminateSession on the agent side.
		_ = c.TerminateSession()
		_ = c.Close()
	}()

	// use a signal handler vs. defer since defer operates after an escape from the outer loop
	// and we can't trust the data channel connection state at that point.  Intercepting signals
	// means we're probably trying to shutdown somewhere in the outer loop, and there's a good
	// possibility that the data channel is still valid
	installSignalHandler(c)

	return startPortForwardingSession(context.Background(), c, opts)
}

// startPortForwardingSession is shared by PortForwardingSession and PortForwardingSessionWithContext
// and routes to either multiplexed or basic port forwarding based on agent version.
func startPortForwardingSession(ctx context.Context, c *datachannel.SsmDataChannel, opts *PortForwardingInput) error {
	if err := c.WaitForHandshakeComplete(ctx); err != nil {
		return err
	}

	agentVersion := c.AgentVersion()

	// Agent version 3.0.196.0+ supports multiplexed port forwarding
	if agentVersionGte(agentVersion, "3.0.196.0") {
		zap.S().Infof("using multiplexed port forwarding (agent version: %s)", agentVersion)
		return startMuxPortForwardingSession(ctx, c, opts)
	}

	zap.S().Infof("using basic port forwarding (agent version: %s)", agentVersion)
	return startBasicPortForwardingSession(ctx, c, opts)
}

// startMuxPortForwardingSession handles multiplexed port forwarding for modern SSM agents.
func startMuxPortForwardingSession(ctx context.Context, c *datachannel.SsmDataChannel, opts *PortForwardingInput) error {
	// Create listener without connection limit for multiplexed mode
	lsnr, err := createListenerUnlimited(opts.LocalPort)
	if err != nil {
		return err
	}
	defer lsnr.Close()

	if opts.ReadyCh != nil {
		close(opts.ReadyCh)
	}

	go func() {
		<-ctx.Done()
		lsnr.Close()
	}()

	log.Printf("listening on %s", lsnr.Addr())

	return startMuxPortForwarding(ctx, c, lsnr, c.AgentVersion())
}

// startBasicPortForwardingSession handles legacy single-connection port forwarding.
//
//nolint:funlen,gocognit // it's long, but not overly hard to read despite what the gocognit says
func startBasicPortForwardingSession(ctx context.Context, c *datachannel.SsmDataChannel, opts *PortForwardingInput) error {
	// Create listener with single connection limit for basic mode
	lsnr, err := createListener(opts.LocalPort)
	if err != nil {
		return err
	}

	closeFunc := &sync.Once{}
	closeLsnr := func() {
		closeFunc.Do(func() { _ = lsnr.Close() })
	}
	defer closeLsnr()

	if opts.ReadyCh != nil {
		close(opts.ReadyCh)
	}

	go func() {
		<-ctx.Done()
		lsnr.Close()
	}()

	log.Printf("listening on %s", lsnr.Addr())

	doneCh := make(chan bool)
	errCh := make(chan error)
	inCh := messageChannel(c, errCh)

outer:
	for {
		var conn net.Conn
		conn, err = lsnr.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				break outer // Expected error due to shutdown
			default:
				log.Print(err)
				continue
			}
		}

		go func() {
			// handle incoming messages from AWS in the background
			if _, e := io.Copy(c, conn); e != nil {
				errCh <- e
			}
			doneCh <- true
		}()

	inner:
		for {
			select {
			case <-doneCh:
				// basic (non-muxing) connections support DisconnectPort to signal to the remote agent that
				// we are shutting down this particular connection on our end, and possibly expect a new one.
				_ = c.DisconnectPort()
				break inner
			case data, ok := <-inCh:
				if !ok {
					// incoming websocket channel is closed, which is fatal
					closeLsnr()
					break outer
				}

				if _, err = conn.Write(data); err != nil {
					zap.S().Info(err)
				}
			case er, ok := <-errCh:
				if !ok {
					// I can't think of a good reason why we'd ever end up here, but if we do
					// we should stop the world
					zap.S().Info("errCh closed")
					closeLsnr()
					break outer
				}

				// any write to errCh means at least 1 of the goroutines has exited
				zap.S().Info(er)
				break inner
			case <-ctx.Done():
				closeLsnr()
				break outer
			}
		}

		closeLsnr()
	}
	return nil
}

// PortPluginSession delegates the execution of the SSM port forwarding to the AWS-managed session manager plugin code,
// bypassing this libraries internal websocket code and connection management.
func PortPluginSession(cfg aws.Config, opts *PortForwardingInput) error {
	documentName := "AWS-StartPortForwardingSession"
	parameters := map[string][]string{
		"localPortNumber": {strconv.Itoa(opts.LocalPort)},
		"portNumber":      {strconv.Itoa(opts.RemotePort)},
	}

	if opts.Host != "" {
		parameters["host"] = []string{opts.Host}
		documentName = "AWS-StartPortForwardingSessionToRemoteHost"
	}

	in := &ssm.StartSessionInput{
		DocumentName: aws.String(documentName),
		Target:       aws.String(opts.Target),
		Parameters:   parameters,
		Reason:       aws.String("ssm-session-client"),
	}

	return PluginSession(cfg, in)
}

func openDataChannel(cfg aws.Config, opts *PortForwardingInput) (*datachannel.SsmDataChannel, error) {
	documentName := "AWS-StartPortForwardingSession"
	parameters := map[string][]string{
		"localPortNumber": {strconv.Itoa(opts.LocalPort)},
		"portNumber":      {strconv.Itoa(opts.RemotePort)},
	}

	// Use remote host document if a host is specified
	if opts.Host != "" {
		documentName = "AWS-StartPortForwardingSessionToRemoteHost"
		parameters["host"] = []string{opts.Host}
	}

	in := &ssm.StartSessionInput{
		DocumentName: aws.String(documentName),
		Target:       aws.String(opts.Target),
		Parameters:   parameters,
	}

	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, in, &datachannel.SSMMessagesResover{
		Endpoint: config.Flags().SSMMessagesVpcEndpoint,
	}); err != nil {
		return nil, err
	}
	return c, nil
}

// read messages from websocket and write payload to the returned channel.
func messageChannel(c datachannel.DataChannel, errCh chan error) chan []byte {
	inCh := make(chan []byte)

	buf := make([]byte, 4096)
	var payload []byte

	go func() {
		defer close(inCh)

		for {
			nr, err := c.Read(buf)
			if err != nil {
				errCh <- err
				return
			}

			payload, err = c.HandleMsg(buf[:nr])
			if err != nil {
				errCh <- err
				return
			}

			if len(payload) > 0 {
				inCh <- payload
			}
		}
	}()

	return inCh
}

// createListener creates a TCP listener limited to a single connection (for basic port forwarding).
func createListener(port int) (net.Listener, error) {
	l, err := net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}

	// Limit to single connection for basic (non-multiplexed) mode
	return netutil.LimitListener(l, 1), nil
}

// createListenerUnlimited creates a TCP listener without connection limits (for multiplexed port forwarding).
func createListenerUnlimited(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
}

// shared with ssh.go.
func installSignalHandler(c datachannel.DataChannel) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		zap.S().Infof("Got signal: %s, shutting down", sig.String())

		_ = c.TerminateSession()
		_ = c.Close()

		os.Exit(0)
	}()
}
