package ssmclient

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ssm"
	"golang.org/x/sys/unix"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
)

// todo
// figure out how to disable input echoing (something like `stty -echo`)
// figure out how to pass tab input for enabling command completion
func ShellSession(cfg client.ConfigProvider, target string) error {
	c := new(dataChannel)
	if err := c.Open(cfg, new(ssm.StartSessionInput).SetTarget(target)); err != nil {
		return err
	}
	defer c.Close()

	inCh, errCh := c.ReaderChannel() // reads message from websocket

	// immediately send SIGWINCH after initializing signal handlers to set terminal size
	installShellSignalHandlers(c) <- syscall.SIGWINCH

	outCh := writePump(os.Stdin, errCh)

outer:
	for {
		select {
		case dataIn, ok := <-inCh:
			if ok {
				_, _ = fmt.Fprintf(os.Stdout, "%s", dataIn) // maybe allow other writers?
			} else {
				// incoming websocket channel is closed, or ChannelClosed message received, time to exit
				close(outCh)
				break outer
			}
		case dataOut, ok := <-outCh:
			if ok {
				if _, err := c.Write(dataOut); err != nil {
					log.Printf("error writing to data channel: %v", err)
				}
			} else {
				// we have lost stdin, all we can do is bail out
				close(inCh)
				break outer
			}
		case err, ok := <-errCh:
			if ok {
				log.Printf("data channel error: %v", err)
			} else {
				// I can't think of a good reason why we'd ever end up here, but if we do
				// we should stop the world
				log.Print("errCh closed")
				close(inCh)
				close(outCh)
				break outer
			}
		}
	}

	return nil
}

func installShellSignalHandlers(c DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGWINCH)
	go func() {
		switch sig := <-sigCh; {
		case sig == syscall.SIGWINCH:
			// fixme bad news ... this doesn't seem to fire after resizing the terminal app window
			// plus, does go implement sigwinch internally for windows (we know the OS proper doesn't)
			if err := updateTermSize(c); err != nil {
				// todo handle error
			}
		default:
			// I think this is all we have to do for shell sessions (don't see any mention of TerminateSession)
			// unlike when we execute the plugin, we can't just ignore these signals and pass them down to the
			// child process (since there isn't one).  We should be able to handle these here, and start closing up
			// fixme - however, it seems that these aren't getting picked up by either this process, or the ssm shell
			log.Printf("Got signal: %s, shutting down", sig.String())
			os.Exit(0)
		}
	}()
	return sigCh
}

func updateTermSize(c DataChannel) error {
	// some contrived default values
	var cols uint32 = 132
	var rows uint32 = 45

	// use the provided ioctl on non-windows platforms to get the terminal size
	// windows will just use default ... until someone can figure that out
	if !strings.EqualFold(runtime.GOOS, "windows") {
		sz, err := unix.IoctlGetWinsize(int(os.Stdin.Fd()), syscall.TIOCGWINSZ)
		if err != nil {
			log.Printf("getwinsize ioctl failed: %v", err)
		} else {
			cols = uint32(sz.Col)
			rows = uint32(sz.Row)
		}
	}

	return c.SetTerminalSize(rows, cols)
}
