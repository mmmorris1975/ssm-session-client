package ssmclient

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ShellPluginSession delegates the execution of the SSM shell session to the AWS-managed session manager plugin code,
// bypassing this libraries internal websocket code and session management.
func ShellPluginSession(ctx context.Context, cfg aws.Config, target string) error {
	return PluginSession(ctx, cfg, &ssm.StartSessionInput{Target: aws.String(target)})
}
