package ssmclient

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"log"
	"os"
	"ssm-session-client/datachannel"
	"strconv"
	"time"
)

// The SSH session client is a specialized port forwarding client where the remote port is defaulted to 22
// and it listens for data from Stdin and sends output to Stdout
// todo - will we need to modify stdin/stdout so it can deal with the ssh traffic?
func SshSession(cfg client.ConfigProvider, opts *PortForwardingInput) error {
	var port = "22"
	if opts.RemotePort > 0 {
		port = strconv.Itoa(opts.RemotePort)
	}

	in := &ssm.StartSessionInput{
		DocumentName: aws.String("AWS-StartSSHSession"),
		Target:       aws.String(opts.Target),
		Parameters: map[string][]*string{
			"portNumber": {aws.String(port)},
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

	installSignalHandler(c)
	time.Sleep(166667 * time.Microsecond) // be sure handshake completes before sending data

	outCh := writePump(os.Stdin, errCh)

	var exiting bool
outer:
	for {
		select {
		case dataIn, ok := <-inCh:
			if ok {
				_, _ = fmt.Fprintf(os.Stdout, "%s", dataIn)
			} else {
				// incoming websocket channel is closed, or ChannelClosed message received, time to exit
				if !exiting {
					close(outCh)
					exiting = true
				}
				break outer
			}
		case dataOut, ok := <-outCh:
			if ok {
				if _, err := c.Write(dataOut); err != nil {
					log.Printf("error writing to data channel: %v", err)
				}
			} else {
				// we have lost stdin, all we can do is bail out
				if !exiting {
					close(inCh)
					exiting = true
				}
				break outer
			}
		case err, ok := <-errCh:
			if ok {
				log.Printf("data channel error: %v", err)
			} else {
				// I can't think of a good reason why we'd ever end up here, but if we do
				// we should stop the world
				log.Print("errCh closed")
				if !exiting {
					close(inCh)
					close(outCh)
					exiting = true
				}
				break outer
			}
		}
	}
	return nil
}
