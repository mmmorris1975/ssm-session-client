// +build !windows

package ssmclient

import (
	"golang.org/x/sys/unix"
	"log"
	"os"
	"os/signal"
	"ssm-session-client/datachannel"
	"syscall"
)

var origTermios *unix.Termios

func initialize(c datachannel.DataChannel) error {
	// configure signal handlers and immediately trigger a size update
	installSignalHandlers(c) <- syscall.SIGWINCH
	return configureStdin()
}

func cleanup() error {
	if origTermios != nil {
		// reset Stdin to original settings
		return unix.IoctlSetTermios(int(os.Stdin.Fd()), syscall.TIOCSETAF, origTermios)
	}
	return nil
}

func configureStdin() (err error) {
	origTermios, err = unix.IoctlGetTermios(int(os.Stdin.Fd()), syscall.TIOCGETA)
	if err != nil {
		return err
	}

	// unsetting ISIG means that this process will no longer respond to the INT, QUIT, SUSP
	// signals (they go downstream to the instance session, which is desirable).  Which means
	// those signals are unavailable for shutting down this process
	newTermios := *origTermios
	newTermios.Iflag = origTermios.Iflag | syscall.IUTF8
	newTermios.Lflag = origTermios.Lflag ^ syscall.ICANON ^ syscall.ECHO ^ syscall.ISIG

	return unix.IoctlSetTermios(int(os.Stdin.Fd()), syscall.TIOCSETAF, &newTermios)
}

func installSignalHandlers(c datachannel.DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 1)

	// we're configuring stdin to pass SIGINT and SIGQUIT to the session terminal, which
	// means they'll never be seen here and there's no use to handle them.
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGWINCH)

	go func() {
		switch <-sigCh {
		case syscall.SIGWINCH:
			// some terminal applications may not fire this signal when resizing (don't see it on MacOS) :(
			// plus, does Go implement sigwinch internally for windows? (we know the OS proper doesn't)
			if err := updateTermSize(c); err != nil {
				// todo handle error (datachannel.SetTerminalSize error)
			}
		case syscall.SIGTERM:
			log.Print("term")
			_ = cleanup()
			// os.Exit(0) ??
		}
	}()

	return sigCh
}

func getWinSize() (rows, cols uint32, err error) {
	var sz *unix.Winsize

	sz, err = unix.IoctlGetWinsize(int(os.Stdin.Fd()), syscall.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}

	return uint32(sz.Row), uint32(sz.Col), nil
}
