// +build !windows !js

package ssmclient

import (
	"golang.org/x/sys/unix"
	"log"
	"os"
	"os/signal"
	"ssm-session-client/datachannel"
)

var origTermios *unix.Termios

func initialize(c datachannel.DataChannel) error {
	// configure signal handlers and immediately trigger a size update
	installSignalHandlers(c) <- unix.SIGWINCH
	return configureStdin()
}

func installSignalHandlers(c datachannel.DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 1)

	// we're configuring stdin to pass SIGINT and SIGQUIT to the session terminal, which
	// means they'll never be seen here and there's no use to handle them.
	signal.Notify(sigCh, unix.SIGTERM, unix.SIGWINCH)

	go func() {
		switch <-sigCh {
		case unix.SIGWINCH:
			// some terminal applications may not fire this signal when resizing (don't see it on MacOS) :(
			// plus, does Go implement sigwinch internally for windows? (we know the OS proper doesn't)
			if err := updateTermSize(c); err != nil {
				// todo handle error (datachannel.SetTerminalSize error)
			}
		case unix.SIGTERM:
			log.Print("term")
			_ = cleanup()
			// os.Exit(0) ??
		}
	}()

	return sigCh
}

func getWinSize() (rows, cols uint32, err error) {
	var sz *unix.Winsize

	sz, err = unix.IoctlGetWinsize(int(os.Stdin.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}

	return uint32(sz.Row), uint32(sz.Col), nil
}
