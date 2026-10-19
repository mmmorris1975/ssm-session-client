package ssmclient

import (
	"context"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// SSHPluginSession delegates the execution of the SSM SSH integration to the AWS-managed session manager plugin code,
// bypassing this libraries internal websocket code and connection management.
func SSHPluginSession(ctx context.Context, cfg aws.Config, opts *PortForwardingInput) error {
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

	return PluginSession(ctx, cfg, in)
}
