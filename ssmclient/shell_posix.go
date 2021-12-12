// +build !windows,!js

package ssmclient

import (
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/mmmorris1975/ssm-session-client/datachannel"
	"golang.org/x/sys/unix"
)

const (
	ResizeSleepInterval = time.Millisecond * 500
)

var origTermios *unix.Termios

func initialize(c datachannel.DataChannel) error {
	// configure signal handlers and immediately trigger a size update
	installSignalHandlers(c) <- unix.SIGWINCH

	// set handle re-size timer
	handleTerminalResize(c)

	return configureStdin()
}

func installSignalHandlers(c datachannel.DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 10)

	// for some reason we're not seeing INT, QUIT, and TERM signals :(
	signal.Notify(sigCh, os.Interrupt, unix.SIGQUIT, unix.SIGTERM, unix.SIGWINCH)

	go func() {
		switch <-sigCh {
		case unix.SIGWINCH:
			// some terminal applications may not fire this signal when resizing (don't see it on MacOS) :(
			// plus, does Go implement sigwinch internally for windows? (we know the OS proper doesn't)
			_ = updateTermSize(c) // todo handle error? (datachannel.SetTerminalSize error)
		case os.Interrupt, unix.SIGQUIT, unix.SIGTERM:
			log.Print("exiting")
			_ = cleanup()
			_ = c.Close()
			os.Exit(0)
		}
	}()

	return sigCh
}

// see also: https://godoc.org/golang.org/x/crypto/ssh/terminal#GetSize.
func getWinSize() (rows, cols uint32, err error) {
	var sz *unix.Winsize

	sz, err = unix.IoctlGetWinsize(int(os.Stdin.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}

	return uint32(sz.Row), uint32(sz.Col), nil
}

// This approach is inspired by AWS's own client:
// https://github.com/aws/session-manager-plugin/blob/65933d1adf368d1efde7380380a19a7a691340c1/src/sessionmanagerplugin/session/shellsession/shellsession.go#L98-L104
func handleTerminalResize(c datachannel.DataChannel) {
	go func() {
		for {
			_ = updateTermSize(c)
			// repeating this loop for every 500ms
			time.Sleep(ResizeSleepInterval)
		}
	}()
}
