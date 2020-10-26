package main

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
	"os"
	"ssm-session-client/ssmclient"
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
		if len(os.Args) > 1 {
			profile = os.Args[1]
			target = os.Args[2]
		}
	}

	if _, ok := os.LookupEnv("AWS_REGION"); !ok {
		_ = os.Setenv("AWS_REGION", "us-east-1")
	}

	s := session.Must(session.NewSessionWithOptions(session.Options{Profile: profile}))
	log.Fatal(ssmclient.ShellSession(s, target))
}
