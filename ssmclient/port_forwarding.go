package ssmclient

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"golang.org/x/net/netutil"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"ssm-session-client/datachannel"
	"strconv"
	"syscall"
)

type PortForwardingInput struct {
	Target     string
	RemotePort int
	LocalPort  int
}

// Both the basic and muxing plugins on the agent side support the Flag payload type with the
// PayloadTypeFlag of TerminateSession.  The basic plugin also supports the DisconnectToPort PayloadTypeFlag
func PortForwardingSession(cfg client.ConfigProvider, opts *PortForwardingInput) error {
	in := &ssm.StartSessionInput{
		DocumentName: aws.String("AWS-StartPortForwardingSession"),
		Target:       aws.String(opts.Target),
		Parameters: map[string][]*string{
			"localPortNumber": {aws.String(strconv.Itoa(opts.LocalPort))},
			"portNumber":      {aws.String(strconv.Itoa(opts.RemotePort))},
		},
	}

	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, in); err != nil {
		return err
	}
	defer func() {
		_ = c.TerminateSession()
		_ = c.Close()
	}()

	inCh, errCh := c.ReaderChannel() // reads message from websocket

	// use a signal handler vs. defer since defer operates after an escape from the outer loop
	// and we can't trust the data channel connection state at that point.  Intercepting signals
	// means we're probably trying to shutdown somewhere in the outer loop, and there's a good
	// possibility that the data channel is still valid
	installSignalHandler(c)

	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(opts.LocalPort)))
	if err != nil {
		return err
	}

	// use limit listener for now, eventually maybe we'll add muxing
	// REF: https://github.com/aws/amazon-ssm-agent/blob/master/agent/session/plugins/port/port_mux.go
	lsnr := netutil.LimitListener(l, 1)
	defer lsnr.Close()
	log.Printf("listening on %s", lsnr.Addr())

outer:
	for {
		var conn net.Conn
		conn, err = lsnr.Accept()
		if err != nil {
			// not fatal, just wait for next
			log.Print(err)
			continue
		}

		outCh := writePump(conn, errCh)

	inner:
		for {
			select {
			case dataIn, ok := <-inCh:
				if ok {
					if _, err = conn.Write(dataIn); err != nil {
						log.Printf("error reading from data channel: %v", err)
					}
				} else {
					// incoming websocket channel is closed, which is fatal
					close(outCh)
					_ = conn.Close()
					break outer
				}
			case dataOut, ok := <-outCh:
				if ok {
					if _, err = c.Write(dataOut); err != nil {
						log.Printf("error writing to data channel: %v", err)
					}
				} else {
					// local TCP connection is closed, should be OK to just break inner loop
					// send DisconnectPort when using non-muxing connection
					if err := c.DisconnectPort(); err != nil {
						log.Printf("disconnect error: %v", err)
					}
					break inner
				}
			case err, ok := <-errCh:
				if ok {
					log.Printf("data channel error: %v", err)
				} else {
					// I can't think of a good reason why we'd ever end up here, but if we do
					// we should stop the world
					log.Print("errCh closed")
					close(inCh)
					close(outCh)
					_ = conn.Close()
					break outer
				}
			}
		}
		_ = conn.Close()
	}

	return nil
}

// shared with shell.go
func writePump(r io.Reader, errCh chan error) chan []byte {
	dataCh := make(chan []byte, 65535)
	buf := make([]byte, 1024)

	go func() {
		for {
			n, err := r.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					// local listener has shut down, there's no more work for us to do on this connection
					close(dataCh)
					break
				} else {
					errCh <- err
					break
				}
			}

			dataCh <- buf[:n]
		}
	}()

	return dataCh
}

func installSignalHandler(c datachannel.DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Got signal: %s, shutting down", sig.String())

		if err := c.TerminateSession(); err != nil {
			log.Printf("error sending TerminateSession: %v", err)
		}

		os.Exit(0)
	}()
	return sigCh
}
