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
	"ssm-session-client/datachannel"
	"strings"
	"syscall"
)

// todo
// figure out how to disable input echoing (something like `stty -echo`) and enable control character handling ^C and arrow keys for shell history navigation
// figure out how to pass tab input for enabling command completion
func ShellSession(cfg client.ConfigProvider, target string) error {
	c := new(datachannel.SsmDataChannel)
	if err := c.Open(cfg, new(ssm.StartSessionInput).SetTarget(target)); err != nil {
		return err
	}
	defer c.Close()

	inCh, errCh := c.ReaderChannel() // reads message from websocket

	// immediately send SIGWINCH after initializing signal handlers to set terminal size
	installShellSignalHandlers(c) <- syscall.SIGWINCH
	//c.Write([]byte("/bin/stty -echo\n"))

	// this probably isn't great anyway, as it assume a *nix system
	//c.Write([]byte{0x1B, 0x5B, 0x35, 0x69, 0x07}) // send terminal escape sequence to disable echo fixme this isn't correct --- with or without trailing BEL (0x07)
	outCh := writePump(os.Stdin, errCh)

	var exiting bool
outer:
	for {
		select {
		case dataIn, ok := <-inCh:
			if ok {
				// don't spit on '\n' to attempt to remove first bit of input ...
				//  A - 1st returned packet may not be the full output (or even the input command) so you'll still see some data echoed
				//  B - it messes up multi-line output coming in via multiple packets, since it's discarding data
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

func installShellSignalHandlers(c datachannel.DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGWINCH)
	go func() {
		switch sig := <-sigCh; {
		case sig == syscall.SIGWINCH:
			// fixme bad news ... this doesn't seem to fire after resizing the terminal app window
			// plus, does go implement sigwinch internally for windows? (we know the OS proper doesn't)
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

func updateTermSize(c datachannel.DataChannel) error {
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
