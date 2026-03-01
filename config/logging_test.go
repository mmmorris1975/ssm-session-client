package config

import (
	"runtime"
	"testing"
)

func TestCreateLogger(t *testing.T) {
	logger, err := CreateLogger()
	if err != nil {
		t.Fatalf("CreateLogger() error: %v", err)
	}
	if logger == nil {
		t.Fatal("CreateLogger() returned nil logger")
	}
}

func TestSetLogLevel_Valid(t *testing.T) {
	// Must call CreateLogger first to initialize logConfig
	_, err := CreateLogger()
	if err != nil {
		t.Fatalf("CreateLogger() error: %v", err)
	}

	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			if err := SetLogLevel(level); err != nil {
				t.Errorf("SetLogLevel(%q) error: %v", level, err)
			}
		})
	}
}

func TestSetLogLevel_Invalid(t *testing.T) {
	// Must call CreateLogger first to initialize logConfig
	_, err := CreateLogger()
	if err != nil {
		t.Fatalf("CreateLogger() error: %v", err)
	}

	if err := SetLogLevel("invalid_level"); err == nil {
		t.Error("SetLogLevel('invalid_level') should return error")
	}
}

func TestGetLogFolder(t *testing.T) {
	folder, err := getLogFolder()
	if err != nil {
		t.Fatalf("getLogFolder() error: %v", err)
	}
	if folder == "" {
		t.Error("getLogFolder() returned empty string")
	}

	switch runtime.GOOS {
	case "darwin":
		if folder == "" {
			t.Error("expected non-empty folder for darwin")
		}
	case "windows":
		if folder == "" {
			t.Error("expected non-empty folder for windows")
		}
	default:
		if folder == "" {
			t.Error("expected non-empty folder for linux")
		}
	}
}
