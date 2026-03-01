package session

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/ssmclient"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

// StartEC2InstanceConnect starts a SSH session using EC2 Instance Connect.
func StartEC2InstanceConnect(target string) error {
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

	pubKey, err := config.FindSSHPublicKey()
	if err != nil {
		zap.S().Fatal(err)
	}

	ec2iccfg, err := BuildAWSConfig(context.Background(), "ec2ic")
	if err != nil {
		zap.S().Fatal(err)
	}

	if err := SendInstanceConnectKey(context.Background(), ec2iccfg, tgt, user, pubKey); err != nil {
		zap.S().Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
	}
	ssmMessagesCfg, err := BuildAWSConfig(context.Background(), "ssmmessages")
	if err != nil {
		zap.S().Fatal(err)
	}
	if config.Flags().UseSSMSessionPlugin {
		return ssmclient.SSHPluginSession(ssmMessagesCfg, &in)
	}
	return ssmclient.SSHSession(ssmMessagesCfg, &in)
}

// SendInstanceConnectKey pushes a temporary SSH public key to an EC2 instance via
// EC2 Instance Connect. The key is valid for 60 seconds, which is enough time to
// establish a connection.
func SendInstanceConnectKey(ctx context.Context, cfg aws.Config, instanceID, user, pubKeyContent string) error {
	ec2i := ec2instanceconnect.NewFromConfig(cfg)
	_, err := ec2i.SendSSHPublicKey(ctx, &ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:     aws.String(instanceID),
		InstanceOSUser: aws.String(user),
		SSHPublicKey:   aws.String(pubKeyContent),
	})
	if err != nil {
		return fmt.Errorf("EC2 Instance Connect SendSSHPublicKey: %w", err)
	}
	zap.S().Infof("EC2 Instance Connect: pushed temporary public key for user %q on instance %s", user, instanceID)
	return nil
}

// GenerateEphemeralSSHKey creates a throwaway Ed25519 key pair in memory.
// The returned signer is used directly for SSH authentication; the public key
// string (authorized_keys format) is what gets pushed via EC2 Instance Connect.
func GenerateEphemeralSSHKey() (signer ssh.Signer, pubKeyAuthorizedFormat string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("generating ephemeral Ed25519 key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, "", fmt.Errorf("converting public key to SSH format: %w", err)
	}

	signer, err = ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, "", fmt.Errorf("creating SSH signer: %w", err)
	}

	pubKeyAuthorizedFormat = string(ssh.MarshalAuthorizedKey(sshPub))
	return signer, pubKeyAuthorizedFormat, nil
}
