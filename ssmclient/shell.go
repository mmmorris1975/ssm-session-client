package ssmclient

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/mmmorris1975/ssm-session-client/datachannel"
	"io"
	"log"
	"os"
)

// ShellSession starts a shell session with the instance specified in the target parameter.  The
// client.ConfigProvider parameter will be used to call the AWS SSM StartSession API, which is used
// as part of establishing the websocket communication channel.
func ShellSession(cfg client.ConfigProvider, target string) error {
	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, new(ssm.StartSessionInput).SetTarget(target)); err != nil {
		return err
	}
	defer c.Close()

	// do platform-specific setup ... signal handling, stdin modification, etc...
	if err := initialize(c); err != nil {
		log.Fatal(err)
	}
	defer cleanup() // platform-specific cleanup, not called if terminated by a signal

	errCh := make(chan error, 5)
	go func() {
		if _, err := io.Copy(c, os.Stdin); err != nil {
			errCh <- err
		}
	}()

	if _, err := io.Copy(os.Stdout, c); err != nil {
		if !errors.Is(err, io.EOF) {
			errCh <- err
		}
		close(errCh)
	}

	return <-errCh
}

func updateTermSize(c datachannel.DataChannel) error {
	rows, cols, err := getWinSize()
	if err != nil {
		log.Printf("getWinSize() failed: %v", err)
	}

	// make sure we set some default terminal size with contrived values
	if rows < 1 {
		rows = 45
	}

	if cols < 1 {
		cols = 132
	}

	//log.Printf("sending set size: rows: %d, cols: %d", rows, cols)
	return c.SetTerminalSize(rows, cols)
}
