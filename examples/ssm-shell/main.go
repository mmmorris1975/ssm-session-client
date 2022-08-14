package main

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/mmmorris1975/ssm-session-client/ssmclient"
	"log"
	"os"
)

// Start a SSM port forwarding session.
// Usage: port-forwarder [profile_name] target
//   The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//   if unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//   set via environment variables, or from the default profile.
//
//   The target parameter is the EC2 instance ID

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

	tgt, err := ssmclient.ResolveTarget(target, cfg)
	if err != nil {
		log.Fatal(err)
	}

	// A 3rd argument can be passed to specify a command to run before turning the shell over to the user
	// Alternatively, can be called as ssmclient.ShellPluginSession(cfg, tgt) to use the AWS-managed SSM session client code
	log.Fatal(ssmclient.ShellSession(cfg, tgt))
}
