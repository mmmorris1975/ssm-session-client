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

	// use a signal handler vs. defer since defer operates after an escape from the outer loop
	// and we can't trust the data channel connection state at that point.  Intercepting signals
	// means we're probably trying to shutdown somewhere in the outer loop, and there's a good
	// possibility that the data channel is still valid
	installSignalHandler(c)

	log.Print("waiting for handshake")
	if err := c.WaitForHandshakeComplete(); err != nil {
		return err
	}

	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(opts.LocalPort)))
	if err != nil {
		return err
	}

	// use limit listener for now, eventually maybe we'll add muxing
	// REF: https://github.com/aws/amazon-ssm-agent/blob/master/agent/session/plugins/port/port_mux.go
	lsnr := netutil.LimitListener(l, 1)
	defer lsnr.Close()
	log.Printf("listening on %s", lsnr.Addr())

	errCh := make(chan error, 5)
	for {
		var conn net.Conn
		conn, err = lsnr.Accept()
		if err != nil {
			// not fatal, just wait for next (maybe unless lsnr is dead?)
			log.Print(err)
			continue
		}

		go func() {
			// uses c.ReadFrom()
			if _, err := io.Copy(c, conn); err != nil {
				errCh <- err
			}
			conn.Close()
			errCh <- nil
		}()

		go func() {
			// uses c.WriteTo()
			if _, err := io.Copy(conn, c); err != nil {
				if !errors.Is(err, io.EOF) {
					errCh <- err
				}
			}
			conn.Close()
			errCh <- nil
		}()

		<-errCh
		//conn.Close()
	}
}

// shared with ssh.go
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
