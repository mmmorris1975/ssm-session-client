package ssmclient

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/datachannel"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"go.uber.org/zap"
)

// SSHSession starts a specialized port forwarding session to allow SSH connectivity to the target instance over
// the SSM session.  It listens for data from Stdin and sends output to Stdout.  Like a port forwarding session,
// use a PortForwardingInput type to configure the session properties.  Any LocalPort information is ignored, and
// if no RemotePort is specified, the default SSH port (22) will be used. The aws.Config parameter is used to call
// the AWS SSM StartSession API, which is used as part of establishing the websocket communication channel.
func SSHSession(cfg aws.Config, opts *PortForwardingInput) error {
	// Ignore SIGPIPE so writes to a broken stdout pipe return errors
	// instead of killing the process. This is critical when used as an
	// SSH ProxyCommand: if SSH exits first, our next write to stdout
	// would trigger SIGPIPE and silently kill us before we can log.
	signal.Ignore(syscall.SIGPIPE)

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
	if err := c.Open(cfg, in, &datachannel.SSMMessagesResover{
		Endpoint: config.Flags().SSMMessagesVpcEndpoint,
	}); err != nil {
		return err
	}
	installSignalHandler(c)

	zap.S().Info("waiting for handshake")
	if err := c.WaitForHandshakeComplete(context.Background()); err != nil {
		return err
	}
	zap.S().Info("handshake complete")

	// Copy stdin to websocket in the background. This goroutine does NOT
	// control session lifetime — stdin may close (e.g. VS Code SSH process
	// management) while the tunnel must remain active for ongoing traffic.
	go func() {
		_, err := io.Copy(c, os.Stdin)
		if err != nil {
			zap.S().Infof("stdin->websocket error: %v", err)
		} else {
			zap.S().Info("stdin->websocket: stdin closed (EOF)")
		}
	}()

	// Copy websocket to stdout in the foreground. When the remote side
	// closes the channel, this returns and we shut down the session.
	_, err := io.Copy(os.Stdout, c)
	if err != nil && !errors.Is(err, io.EOF) {
		zap.S().Infof("websocket->stdout error: %v", err)
	} else {
		zap.S().Info("websocket->stdout: channel closed (EOF)")
	}

	_ = c.TerminateSession()
	_ = c.Close()

	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// SSHPluginSession delegates the execution of the SSM SSH integration to the AWS-managed session manager plugin code,
// bypassing this libraries internal websocket code and connection management.
func SSHPluginSession(cfg aws.Config, opts *PortForwardingInput) error {
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
		Reason: aws.String("ssm-session-client"),
	}

	return PluginSession(cfg, in)
}
