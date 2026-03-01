package session

import (
	"context"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// StartSSMPortForwarder starts a port forwarding session using AWS SSM.
func StartSSMPortForwarder(target string, sourcePort int) error {
	_, host, port, err := ParseHostPort(target, "", 22)
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

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
		LocalPort:  sourcePort,
	}
	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.PortPluginSession(ssmMessagesCfg, &in)
	}
	return ssmclient.PortForwardingSession(ssmMessagesCfg, &in)
}
