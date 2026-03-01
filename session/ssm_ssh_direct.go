package session

import (
	"context"
	"fmt"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"go.uber.org/zap"
)

// StartSSHDirectSession starts a direct SSH session to the target EC2 instance
// via AWS SSM without requiring an external SSH client.
func StartSSHDirectSession(target string) error {
	user, host, port, err := ParseHostPort(target, "ec2-user", 22)
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

	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}

	opts := &ssmclient.SSHDirectInput{
		Target:         tgt,
		User:           user,
		RemotePort:     port,
		KeyFile:        config.Flags().SSHKeyFile,
		NoHostKeyCheck: config.Flags().NoHostKeyCheck,
		ExecCommand:    config.Flags().SSHExecCommand,
	}

	if config.Flags().UseInstanceConnect && !config.Flags().NoInstanceConnect {
		if err := prepareInstanceConnect(context.Background(), tgt, user, opts); err != nil {
			return err
		}
	}

	return ssmclient.SSHDirectSession(ssmMessagesCfg, opts)
}

// prepareInstanceConnect generates an ephemeral SSH key pair, pushes the public
// half to the EC2 instance via EC2 Instance Connect, and stores the signer on
// opts so it is used as the first authentication method.
func prepareInstanceConnect(ctx context.Context, instanceID, user string, opts *ssmclient.SSHDirectInput) error {
	signer, pubKey, err := GenerateEphemeralSSHKey()
	if err != nil {
		return fmt.Errorf("generating ephemeral key: %w", err)
	}

	ec2iccfg, err := BuildAWSConfig(ctx, "ec2ic")
	if err != nil {
		return fmt.Errorf("building EC2IC config: %w", err)
	}

	if err := SendInstanceConnectKey(ctx, ec2iccfg, instanceID, user, pubKey); err != nil {
		return err
	}

	opts.EphemeralSigner = signer
	return nil
}
