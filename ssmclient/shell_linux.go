// +build linux

package ssmclient

import (
	"golang.org/x/sys/unix"
	"os"
)

func cleanup() error {
	if origTermios != nil {
		// reset Stdin to original settings
		return unix.IoctlSetTermios(int(os.Stdin.Fd()), unix.TCSETSF, origTermios)
	}
	return nil
}

// see also: https://godoc.org/golang.org/x/crypto/ssh/terminal#MakeRaw.
func configureStdin() (err error) {
	origTermios, err = unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}

	// unsetting ISIG means that this process will no longer respond to the INT, QUIT, SUSP
	// signals (they go downstream to the instance session, which is desirable).  Which means
	// those signals are unavailable for shutting down this process
	newTermios := *origTermios
	newTermios.Iflag = origTermios.Iflag | unix.IUTF8
	newTermios.Lflag = origTermios.Lflag ^ unix.ICANON ^ unix.ECHO ^ unix.ISIG

	return unix.IoctlSetTermios(int(os.Stdin.Fd()), unix.TCSETSF, &newTermios)
}
