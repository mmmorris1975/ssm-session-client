package ssmclient

import (
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/mmmorris1975/ssm-session-client/datachannel"
	"io"
	"log"
	"os"
	"strconv"
)

// SSHSession starts a specialized port forwarding session to allow SSH connectivity to the target instance over
// the SSM session.  It listens for data from Stdin and sends output to Stdout.  Like a port forwarding session,
// use a PortForwardingInput type to configure the session properties.  Any LocalPort information is ignored, and
// if no RemotePort is specified, the default SSH port (22) will be used. The client.ConfigProvider parameter is
// used to call the AWS SSM StartSession API, which is used as part of establishing the websocket communication channel.
func SSHSession(cfg aws.Config, opts *PortForwardingInput) error {
	var port = "22"
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
	if err := c.Open(cfg, in); err != nil {
		return err
	}
	defer func() {
		_ = c.TerminateSession()
		_ = c.Close()
	}()

	installSignalHandler(c)

	log.Print("waiting for handshake")
	if err := c.WaitForHandshakeComplete(); err != nil {
		return err
	}
	log.Print("handshake complete")

	errCh := make(chan error, 5)
	go func() {
		if _, err := io.Copy(c, os.Stdin); err != nil {
			errCh <- err
		}
	}()

	if _, err := io.Copy(os.Stdout, c); err != nil {
		if !errors.Is(err, io.EOF) {
			errCh <- err
		}
		close(errCh)
	}

	return <-errCh
}
