package ssmclient

import (
	"context"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// PortForwardingInput configures the port forwarding session parameters.
// Target is the EC2 instance ID to establish the session with.
// RemotePort is the port on the EC2 instance to connect to.
// LocalPort is the port on the local host to listen to.  If not provided, a random port will be used.
type PortForwardingInput struct {
	Target     string
	RemotePort int
	LocalPort  int
	Host       string // optional
}

// PortPluginSession delegates the execution of the SSM port forwarding to the AWS-managed session manager plugin code,
// bypassing this libraries internal websocket code and connection management.
func PortPluginSession(ctx context.Context, cfg aws.Config, opts *PortForwardingInput) error {
	return PluginSession(ctx, cfg, portForwardingSessionInput(opts))
}

func portForwardingSessionInput(opts *PortForwardingInput) *ssm.StartSessionInput {
	documentName := "AWS-StartPortForwardingSession"
	parameters := map[string][]string{
		"localPortNumber": {strconv.Itoa(opts.LocalPort)},
		"portNumber":      {strconv.Itoa(opts.RemotePort)},
	}

	if opts.Host != "" {
		documentName = "AWS-StartPortForwardingSessionToRemoteHost"
		parameters["host"] = []string{opts.Host}
	}

	return &ssm.StartSessionInput{
		DocumentName: aws.String(documentName),
		Target:       aws.String(opts.Target),
		Parameters:   parameters,
	}
}
