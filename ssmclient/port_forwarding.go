package ssmclient

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/mmmorris1975/ssm-session-client/datachannel"
	"golang.org/x/net/netutil"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

// PortForwardingInput configures the port forwarding session parameters.
// Target is the EC2 instance ID to establish the session with.
// RemotePort is the port on the EC2 instance to connect to.
// LocalPort is the port on the local host to listen to.  If not provided, a random port will be used.
type PortForwardingInput struct {
	Target     string
	RemotePort int
	LocalPort  int
}

// PortForwardingSession starts a port forwarding session using the PortForwardingInput parameters to
// configure the session.  The client.ConfigProvider parameter will be used to call the AWS SSM StartSession
// API, which is used as part of establishing the websocket communication channel.
//nolint:funlen,gocognit // it's long, but not overly hard to read despite what the gocognit says
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

	if err = c.WaitForHandshakeComplete(); err != nil {
		return err
	}

	lsnr, err := createListener(opts.LocalPort)
	if err != nil {
		return err
	}
	defer lsnr.Close()
	log.Printf("listening on %s", lsnr.Addr())

	doneCh := make(chan bool)
	errCh := make(chan error)
	inCh := messageChannel(c, errCh)

outer:
	for {
		var conn net.Conn
		conn, err = lsnr.Accept()
		if err != nil {
			// not fatal, just wait for next (maybe unless lsnr is dead?)
			log.Print(err)
			continue
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
					_ = conn.Close()
					break outer
				}

				if _, err = conn.Write(data); err != nil {
					log.Print(err)
				}
			case er, ok := <-errCh:
				if !ok {
					// I can't think of a good reason why we'd ever end up here, but if we do
					// we should stop the world
					log.Print("errCh closed")
					_ = conn.Close()
					break outer
				}

				// any write to errCh means at least 1 of the goroutines has exited
				log.Print(er)
				break inner
			}
		}

		_ = conn.Close()
	}
	return nil
}

func openDataChannel(cfg aws.Config, opts *PortForwardingInput) (*datachannel.SsmDataChannel, error) {
	in := &ssm.StartSessionInput{
		DocumentName: aws.String("AWS-StartPortForwardingSession"),
		Target:       aws.String(opts.Target),
		Parameters: map[string][]string{
			"localPortNumber": {strconv.Itoa(opts.LocalPort)},
			"portNumber":      {strconv.Itoa(opts.RemotePort)},
		},
	}

	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, in); err != nil {
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

func createListener(port int) (net.Listener, error) {
	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}

	// use limit listener for now, eventually maybe we'll add muxing
	// REF: https://github.com/aws/amazon-ssm-agent/blob/master/agent/session/plugins/port/port_mux.go
	return netutil.LimitListener(l, 1), nil
}

// shared with ssh.go.
func installSignalHandler(c datachannel.DataChannel) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Got signal: %s, shutting down", sig.String())

		_ = c.TerminateSession()
		_ = c.Close()

		os.Exit(0)
	}()
}
