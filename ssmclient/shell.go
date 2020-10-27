package ssmclient

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
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

	inCh, errCh := c.ReaderChannel() // reads message from websocket

	// call initialize after doing ReaderChannel (so we process anything we need to do with the websocket)
	// but before we start using Stdin with writePump()
	if err := initialize(c); err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	outCh := writePump(os.Stdin, errCh)

	var exiting bool
outer:
	for {
		select {
		case dataIn, ok := <-inCh:
			if ok {
				_, _ = fmt.Fprintf(os.Stdout, "%s", dataIn) // maybe allow other writers?
			} else {
				// incoming websocket channel is closed, or ChannelClosed message received, time to exit
				if !exiting {
					close(outCh)
					exiting = true
				}
				break outer
			}
		case dataOut, ok := <-outCh:
			if ok {
				if _, err := c.Write(dataOut); err != nil {
					log.Printf("error writing to data channel: %v", err)
				}
			} else {
				// we have lost stdin, all we can do is bail out
				if !exiting {
					close(inCh)
					exiting = true
				}
				break outer
			}
		case err, ok := <-errCh:
			if ok {
				log.Printf("data channel error: %v", err)
			} else {
				// I can't think of a good reason why we'd ever end up here, but if we do
				// we should stop the world
				log.Print("errCh closed")
				if !exiting {
					close(inCh)
					close(outCh)
					exiting = true
				}
				break outer
			}
		}
	}

	return nil
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
