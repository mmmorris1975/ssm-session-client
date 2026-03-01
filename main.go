package main

import (
	"fmt"
	"log"
	"os"

	"github.com/alexbacchin/ssm-session-client/cmd"
	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"go.uber.org/zap"
)

func main() {
	logger, err := config.CreateLogger()
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}
	zap.ReplaceGlobals(logger)
	defer logger.Sync() // flushes buffer, if any

	// When invoked with OpenSSH-style arguments (e.g. via VSCode Remote SSH
	// setting remote.SSH.path), bypass cobra and handle directly.
	if session.IsSSHCompatMode(os.Args) {
		// Handle -V (version query used by VSCode Remote SSH)
		if session.HasVersionFlag(os.Args[1:]) {
			fmt.Fprintln(os.Stdout, "OpenSSH_9.0 ssm-session-client")
			return
		}
		// Load config file and SSC_ environment variables
		cmd.LoadConfig("")
		if err := session.RunSSHCompat(os.Args[1:]); err != nil {
			zap.S().Fatal(err)
		}
		return
	}

	cmd.Execute()
}
