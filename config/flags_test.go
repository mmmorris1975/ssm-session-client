package config

import (
	"testing"
)

func TestFlags_Singleton(t *testing.T) {
	f1 := Flags()
	f2 := Flags()

	if f1 != f2 {
		t.Error("Flags() should return the same pointer")
	}
}

func TestFlags_NotNil(t *testing.T) {
	f := Flags()
	if f == nil {
		t.Fatal("Flags() should not return nil")
	}
}

func TestFlags_DefaultValues(t *testing.T) {
	// Reset singleton for testing
	saved := singleFlags
	singleFlags = Config{}
	defer func() { singleFlags = saved }()

	f := Flags()

	if f.AWSProfile != "" {
		t.Errorf("AWSProfile = %q, want empty", f.AWSProfile)
	}
	if f.AWSRegion != "" {
		t.Errorf("AWSRegion = %q, want empty", f.AWSRegion)
	}
	if f.UseSSMSessionPlugin {
		t.Error("UseSSMSessionPlugin should default to false")
	}
	if f.UseSSOLogin {
		t.Error("UseSSOLogin should default to false")
	}
	if f.SSOOpenBrowser {
		t.Error("SSOOpenBrowser should default to false")
	}
	if f.LogLevel != "" {
		t.Errorf("LogLevel = %q, want empty", f.LogLevel)
	}
}

func TestFlags_Mutation(t *testing.T) {
	saved := singleFlags
	defer func() { singleFlags = saved }()

	f := Flags()
	f.AWSProfile = "test-profile"
	f.AWSRegion = "us-west-2"

	// Changes should be visible through another Flags() call
	f2 := Flags()
	if f2.AWSProfile != "test-profile" {
		t.Errorf("AWSProfile = %q, want %q", f2.AWSProfile, "test-profile")
	}
	if f2.AWSRegion != "us-west-2" {
		t.Errorf("AWSRegion = %q, want %q", f2.AWSRegion, "us-west-2")
	}
}
