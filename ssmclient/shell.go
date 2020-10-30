package ssmclient

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"io"
	"log"
	"os"
	"ssm-session-client/datachannel"
)

func ShellSession(cfg client.ConfigProvider, target string) error {
	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, new(ssm.StartSessionInput).SetTarget(target)); err != nil {
		return err
	}
	defer c.Close()

	// call initialize after doing ReaderChannel (so we process anything we need to do with the websocket)
	// but before we start using Stdin with writePump()
	if err := initialize(c); err != nil {
		log.Fatal(err)
	}
	defer cleanup() // not called if terminated by a signal

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
