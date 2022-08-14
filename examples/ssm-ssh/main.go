package main

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/mmmorris1975/ssm-session-client/ssmclient"
	"log"
	"net"
	"os"
)

// Start a SSM SSH session.
// Usage: ssm-ssh [profile_name] target_spec
//   The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//   if unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//   set via environment variables, or from the default profile.
//
//   The target_spec parameter is required, and is in the form of ec2_instance_id[:port_number] (ex: i-deadbeef:2222)
//   The port_number argument is optional, and if not provided the default SSH port (22) is used.

func main() {
	var profile string
	target := os.Args[1]

	if v, ok := os.LookupEnv("AWS_PROFILE"); ok {
		profile = v
	} else {
		if len(os.Args) > 1 {
			profile = os.Args[1]
			target = os.Args[2]
		}
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithSharedConfigProfile(profile))
	if err != nil {
		log.Fatal(err)
	}

	var port int
	t, p, err := net.SplitHostPort(target)
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

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
	}

	// Alternatively, can be called as ssmclient.SSHPluginSession(cfg, tgt) to use the AWS-managed SSM session client code
	log.Fatal(ssmclient.SSHSession(cfg, &in))
}
