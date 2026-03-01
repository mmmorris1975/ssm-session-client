//go:build windows
// +build windows

package ssmclient

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/alexbacchin/ssm-session-client/datachannel"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

const (
	ResizeSleepInterval = time.Millisecond * 500

	// Windows console mode flags
	ENABLE_ECHO_INPUT              = 0x0004
	ENABLE_LINE_INPUT              = 0x0002
	ENABLE_PROCESSED_INPUT         = 0x0001
	ENABLE_VIRTUAL_TERMINAL_INPUT  = 0x0200
	ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
)

var (
	// Original console modes for restoration on cleanup
	origInMode  uint32
	origOutMode uint32
)

func initialize(c datachannel.DataChannel) error {
	// Setup signal handlers and terminal resize handling
	installSignalHandlers(c)
	handleTerminalResize(c)

	// Configure stdin for raw mode and enable VT processing
	if err := configureStdin(); err != nil {
		return fmt.Errorf("failed to configure stdin: %w", err)
	}

	return nil
}

func configureStdin() error {
	// Get stdin handle
	stdinHandle, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return fmt.Errorf("failed to get stdin handle: %w", err)
	}

	// Save original stdin mode
	if err := windows.GetConsoleMode(stdinHandle, &origInMode); err != nil {
		return fmt.Errorf("failed to get stdin console mode: %w", err)
	}

	// Configure stdin for raw mode with VT input support
	// Clear flags: ECHO_INPUT, LINE_INPUT, PROCESSED_INPUT
	// Set flag: VIRTUAL_TERMINAL_INPUT for escape sequence support
	newInMode := origInMode
	newInMode &^= ENABLE_ECHO_INPUT | ENABLE_LINE_INPUT | ENABLE_PROCESSED_INPUT
	newInMode |= ENABLE_VIRTUAL_TERMINAL_INPUT

	if err := windows.SetConsoleMode(stdinHandle, newInMode); err != nil {
		return fmt.Errorf("failed to set stdin console mode: %w", err)
	}

	// Get stdout handle and enable VT processing for output
	stdoutHandle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return fmt.Errorf("failed to get stdout handle: %w", err)
	}

	// Save original stdout mode
	if err := windows.GetConsoleMode(stdoutHandle, &origOutMode); err != nil {
		return fmt.Errorf("failed to get stdout console mode: %w", err)
	}

	// Enable VT processing for stdout
	newOutMode := origOutMode | ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if err := windows.SetConsoleMode(stdoutHandle, newOutMode); err != nil {
		return fmt.Errorf("failed to set stdout console mode: %w", err)
	}

	return nil
}

func cleanup() error {
	// Restore stdin to original mode
	stdinHandle, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err == nil && origInMode != 0 {
		_ = windows.SetConsoleMode(stdinHandle, origInMode)
	}

	// Restore stdout to original mode
	stdoutHandle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err == nil && origOutMode != 0 {
		_ = windows.SetConsoleMode(stdoutHandle, origOutMode)
	}

	return nil
}

func getWinSize() (rows, cols uint32, err error) {
	//get the size of the console window on windows
	var csbi windows.ConsoleScreenBufferInfo
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return 0, 0, err
	}
	err = windows.GetConsoleScreenBufferInfo(h, &csbi)
	if err != nil {
		return 0, 0, err
	}
	return uint32(csbi.Window.Bottom - csbi.Window.Top + 1), uint32(csbi.Window.Right - csbi.Window.Left + 1), nil

}

func installSignalHandlers(c datachannel.DataChannel) chan os.Signal {
	sigCh := make(chan os.Signal, 10)

	// for some reason we're not seeing INT, QUIT, and TERM signals :(
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		switch <-sigCh {
		case os.Interrupt:
			zap.S().Info("exiting")
			_ = cleanup()
			_ = c.Close()
			os.Exit(0)
		}
	}()

	return sigCh
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
