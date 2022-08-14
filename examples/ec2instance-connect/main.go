package main

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"github.com/mmmorris1975/ssm-session-client/ssmclient"
	"log"
	"net"
	"os"
	"strings"
)

// Start a SSH session. This program is meant to be configured as a ProxyCommand in the ssh_config file.
// Usage: ec2instance-connect [profile] user@target_spec
//   The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//   If unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//   set via environment variables, or from the default profile.
//
//   The user parameter should be set as the user used to connect to the remote host.  This is required by the
//   AWS API in order to provision the SSH public key for the connection.
//
//   The target_spec parameter is required, and is in the form of ec2_instance_id:port_number (ex: i-deadbeef:80)
//
// Example ssh_config :
//   Host i-*
//     IdentityFile ~/.ssh/path_to_your_private_key
//     ProxyCommand ec2instance-connect %r@%h:%p
//     User ec2-user
func main() {
	var profile string
	target := os.Args[1]

	if v, ok := os.LookupEnv("AWS_PROFILE"); ok {
		profile = v
	} else {
		if len(os.Args) > 2 {
			profile = os.Args[1]
			target = os.Args[2]
		}
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithSharedConfigProfile(profile))
	if err != nil {
		log.Fatal(err)
	}

	var port int
	userHost := strings.Split(target, `@`)
	t, p, err := net.SplitHostPort(userHost[1])
	if err == nil {
		port, err = net.LookupPort("tcp", p)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		t = target
	}

	tgt, err := ssmclient.ResolveTarget(t, cfg)
	if err != nil {
		log.Fatal(err)
	}

	ec2i := ec2instanceconnect.NewFromConfig(cfg)
	pubkeyIn := ec2instanceconnect.SendSSHPublicKeyInput{
		InstanceId:     aws.String(tgt),
		InstanceOSUser: aws.String(userHost[0]),
		SSHPublicKey:   aws.String(""), // FIXME - load your SSH public key here
	}
	if _, err = ec2i.SendSSHPublicKey(context.Background(), &pubkeyIn); err != nil {
		log.Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
	}

	// Alternatively, can be called as ssmclient.SSHPluginSession(cfg, tgt) to use the AWS-managed SSM session client code
	log.Fatal(ssmclient.SSHSession(cfg, &in))
}
