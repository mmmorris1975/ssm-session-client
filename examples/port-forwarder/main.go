package main

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/mmmorris1975/ssm-session-client/ssmclient"
	"log"
	"net"
	"os"
	"strings"
)

// Start a SSM port forwarding session.
// Usage: port-forwarder [profile_name] target_spec
//   The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
//   if unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
//   set via environment variables, or from the default profile.
//
//   The target_spec parameter is required, and is in the form of ec2_instance_id:port_number (ex: i-deadbeef:80)

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

	s := session.Must(session.NewSessionWithOptions(
		session.Options{
			Profile:           profile,
			SharedConfigState: session.SharedConfigEnable,
		}))

	parts := strings.Split(target, `:`)

	tgt, err := ssmclient.ResolveTarget(strings.Join(parts[:len(parts)-1], `:`), s)
	if err != nil {
		log.Fatal(err)
	}

	var port int
	port, err = net.LookupPort("tcp", parts[len(parts)-1]) // SSM port forwarding only supports TCP (afaik)
	if err != nil {
		log.Fatal(err)
	}

	in := ssmclient.PortForwardingInput{
		Target:     tgt,
		RemotePort: port,
		LocalPort:  0, // just use random port for demo purposes (this is the default, if not set > 0)
	}
	log.Fatal(ssmclient.PortForwardingSession(s, &in))
}
