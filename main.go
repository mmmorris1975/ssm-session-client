package main

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
	"os"
	"ssm-session-client/ssmclient"
)

func main() {
	in := ssmclient.PortForwardingInput{
		Target:     "i-06c6d8a80657acdd7",
		RemotePort: 53,
		//LocalPort:  0,
	}

	_ = os.Setenv("AWS_REGION", "us-east-2")
	s := session.Must(session.NewSessionWithOptions(session.Options{Profile: "personal"}))
	log.Fatal(ssmclient.PortForwardingSession(s, &in))
}
