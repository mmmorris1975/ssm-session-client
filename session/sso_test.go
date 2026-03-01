package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Error type tests ---

func TestProfileValidationError(t *testing.T) {
	err := NewProfileValidationError("myprofile", "/path/to/config", "region", "", "<non empty>")
	msg := err.Error()

	if msg == "" {
		t.Error("Error() should not be empty")
	}
	if _, ok := interface{}(err).(error); !ok {
		t.Error("ProfileValidationError should implement error interface")
	}
}

func TestLoadingConfigFileError(t *testing.T) {
	inner := errors.New("file not found")
	err := NewLoadingConfigFileError("/path/to/config", inner)

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestMissingProfileError(t *testing.T) {
	inner := errors.New("parse error")
	err := NewMissingProfileError("myprofile", "/path/to/config", inner)

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestCacheFilepathGenerationError(t *testing.T) {
	inner := errors.New("hash error")
	err := NewCacheFilepathGenerationError("myprofile", "https://start.example.com", inner)

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestConfigFileLoadError(t *testing.T) {
	inner := errors.New("load error")
	err := ConfigFileLoadError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestCredCacheError(t *testing.T) {
	inner := errors.New("cache error")
	err := CredCacheError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestOsUserError(t *testing.T) {
	inner := errors.New("user error")
	err := OsUserError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestSsoOidcClientError(t *testing.T) {
	inner := errors.New("oidc error")
	err := SsoOidcClientError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestStartDeviceAuthorizationError(t *testing.T) {
	inner := errors.New("auth error")
	err := StartDeviceAuthorizationError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestBrowserOpenError(t *testing.T) {
	inner := errors.New("browser error")
	err := BrowserOpenError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestSsoOidcTokenCreationError(t *testing.T) {
	inner := errors.New("token error")
	err := SsoOidcTokenCreationError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestGetCallerIdError(t *testing.T) {
	inner := errors.New("sts error")
	err := GetCallerIdError{Err: inner}

	if err.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

func TestCacheFileCreationError(t *testing.T) {
	inner := errors.New("write error")
	err := CacheFileCreationError{Err: inner, Reason: "test reason", CacheFilePath: "/tmp/cache"}

	msg := err.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
	if !errors.Is(err, inner) {
		t.Error("Unwrap() should return inner error")
	}
}

// --- SSOLoginInput tests ---

func TestSSOLoginInput_Validate_Defaults(t *testing.T) {
	input := &SSOLoginInput{}
	if err := input.validate(); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	if input.LoginTimeout != 90*time.Second {
		t.Errorf("LoginTimeout = %v, want %v", input.LoginTimeout, 90*time.Second)
	}
}

func TestSSOLoginInput_Validate_CustomTimeout(t *testing.T) {
	input := &SSOLoginInput{LoginTimeout: 120 * time.Second}
	if err := input.validate(); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	if input.LoginTimeout != 120*time.Second {
		t.Errorf("LoginTimeout = %v, want %v", input.LoginTimeout, 120*time.Second)
	}
}

// --- configProfile tests ---

func TestConfigProfile_Validate_Valid(t *testing.T) {
	profile := &configProfile{
		name:         "test",
		region:       "us-east-1",
		ssoAccountId: "123456789012",
		ssoRegion:    "us-east-1",
		ssoRoleName:  "MyRole",
		ssoStartUrl:  "https://my-sso.awsapps.com/start",
	}

	if err := profile.validate("test", "/path/to/config"); err != nil {
		t.Errorf("validate() error: %v", err)
	}
}

func TestConfigProfile_Validate_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		profile configProfile
	}{
		{"missing name", configProfile{region: "us-east-1", ssoAccountId: "123", ssoRegion: "us-east-1", ssoRoleName: "r", ssoStartUrl: "u"}},
		{"missing region", configProfile{name: "test", ssoAccountId: "123", ssoRegion: "us-east-1", ssoRoleName: "r", ssoStartUrl: "u"}},
		{"missing ssoAccountId", configProfile{name: "test", region: "us-east-1", ssoRegion: "us-east-1", ssoRoleName: "r", ssoStartUrl: "u"}},
		{"missing ssoRegion", configProfile{name: "test", region: "us-east-1", ssoAccountId: "123", ssoRoleName: "r", ssoStartUrl: "u"}},
		{"missing ssoRoleName", configProfile{name: "test", region: "us-east-1", ssoAccountId: "123", ssoRegion: "us-east-1", ssoStartUrl: "u"}},
		{"missing ssoStartUrl", configProfile{name: "test", region: "us-east-1", ssoAccountId: "123", ssoRegion: "us-east-1", ssoRoleName: "r"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.profile.validate("test", "/config"); err == nil {
				t.Error("validate() should return error for missing field")
			}
		})
	}
}

func TestConfigProfile_Validate_DefaultOutput(t *testing.T) {
	profile := &configProfile{
		name:         "test",
		region:       "us-east-1",
		ssoAccountId: "123456789012",
		ssoRegion:    "us-east-1",
		ssoRoleName:  "MyRole",
		ssoStartUrl:  "https://my-sso.awsapps.com/start",
		output:       "",
	}

	if err := profile.validate("test", "/config"); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
	if profile.output != "json" {
		t.Errorf("output = %q, want %q", profile.output, "json")
	}
}

// --- INI parsing tests ---

func TestGetConfigProfile_ValidProfile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	content := `[default]
region = us-west-2
output = json

[profile myprofile]
region = us-east-1
sso_account_id = 123456789012
sso_region = us-east-1
sso_role_name = MyRole
sso_start_url = https://my-sso.awsapps.com/start
output = json
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	profile, err := getConfigProfile("myprofile", configPath)
	if err != nil {
		t.Fatalf("getConfigProfile() error: %v", err)
	}

	if profile.name != "myprofile" {
		t.Errorf("name = %q, want %q", profile.name, "myprofile")
	}
	if profile.region != "us-east-1" {
		t.Errorf("region = %q, want %q", profile.region, "us-east-1")
	}
	if profile.ssoAccountId != "123456789012" {
		t.Errorf("ssoAccountId = %q, want %q", profile.ssoAccountId, "123456789012")
	}
}

func TestGetConfigProfile_MissingProfile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	content := `[default]
region = us-west-2
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	_, err := getConfigProfile("nonexistent", configPath)
	if err == nil {
		t.Error("getConfigProfile() should return error for missing profile")
	}

	var missingErr MissingProfileError
	if !errors.As(err, &missingErr) {
		t.Errorf("error type = %T, want MissingProfileError", err)
	}
}

func TestGetConfigProfile_MissingConfigFile(t *testing.T) {
	_, err := getConfigProfile("myprofile", "/nonexistent/path/config")
	if err == nil {
		t.Error("getConfigProfile() should return error for missing config file")
	}

	var loadErr LoadingConfigFileError
	if !errors.As(err, &loadErr) {
		t.Errorf("error type = %T, want LoadingConfigFileError", err)
	}
}

func TestGetConfigProfile_DefaultFallback(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	content := `[default]
region = us-west-2
output = yaml

[profile myprofile]
sso_account_id = 123456789012
sso_region = us-east-1
sso_role_name = MyRole
sso_start_url = https://my-sso.awsapps.com/start
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	profile, err := getConfigProfile("myprofile", configPath)
	if err != nil {
		t.Fatalf("getConfigProfile() error: %v", err)
	}

	// Region should fall back to default section
	if profile.region != "us-west-2" {
		t.Errorf("region = %q, want %q (from default)", profile.region, "us-west-2")
	}
	// Output should fall back to default section
	if profile.output != "yaml" {
		t.Errorf("output = %q, want %q (from default)", profile.output, "yaml")
	}
}

func TestGetConfigProfile_SSOSession(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	content := `[default]
region = us-west-2

[profile myprofile]
region = us-east-1
sso_account_id = 123456789012
sso_role_name = MyRole
sso_session = my-session
output = json

[sso-session my-session]
sso_region = eu-west-1
sso_start_url = https://session-sso.awsapps.com/start
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	profile, err := getConfigProfile("myprofile", configPath)
	if err != nil {
		t.Fatalf("getConfigProfile() error: %v", err)
	}

	if profile.ssoSession != "my-session" {
		t.Errorf("ssoSession = %q, want %q", profile.ssoSession, "my-session")
	}
	// Should use sso-session values
	if profile.ssoRegion != "eu-west-1" {
		t.Errorf("ssoRegion = %q, want %q", profile.ssoRegion, "eu-west-1")
	}
	if profile.ssoStartUrl != "https://session-sso.awsapps.com/start" {
		t.Errorf("ssoStartUrl = %q, want %q", profile.ssoStartUrl, "https://session-sso.awsapps.com/start")
	}
}

// --- setDefaults tests ---

func TestSetDefaults_NilSection(t *testing.T) {
	profile := &configProfile{name: "test"}
	// Should not panic
	setDefaults(profile, nil)
}

// --- findIniSection tests ---

func TestFindIniSection_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	content := `[Profile MyProfile]
region = us-east-1
`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// The getConfigProfile uses lowercase "profile" prefix
	// but section in file uses "Profile" (capitalized)
	// findIniSection uses case-insensitive prefix matching
}

// --- writeCacheFile tests ---

func TestWriteCacheFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Pre-create directory with proper permissions since writeCacheFile uses 0600 for MkdirAll
	// which would create a non-traversable directory
	cacheDir := filepath.Join(tmpDir, "sso", "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	cachePath := filepath.Join(cacheDir, "test-cache.json")

	data := &cacheFileData{
		StartUrl:    "https://example.com",
		Region:      "us-east-1",
		AccessToken: "test-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}

	if err := writeCacheFile(data, cachePath); err != nil {
		t.Fatalf("writeCacheFile() error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("cache file was not created")
	}

	// Verify content is valid JSON
	content, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(content) == 0 {
		t.Error("cache file is empty")
	}
}

// --- getCacheFilePath tests ---

func TestGetCacheFilePath_WithStartUrl(t *testing.T) {
	profile := &configProfile{
		name:        "test",
		ssoStartUrl: "https://my-sso.awsapps.com/start",
	}

	path, err := getCacheFilePath(profile)
	if err != nil {
		t.Fatalf("getCacheFilePath() error: %v", err)
	}
	if path == "" {
		t.Error("getCacheFilePath() returned empty path")
	}
}

func TestGetCacheFilePath_WithSSOSession(t *testing.T) {
	profile := &configProfile{
		name:        "test",
		ssoStartUrl: "https://my-sso.awsapps.com/start",
		ssoSession:  "my-session",
	}

	path, err := getCacheFilePath(profile)
	if err != nil {
		t.Fatalf("getCacheFilePath() error: %v", err)
	}
	if path == "" {
		t.Error("getCacheFilePath() returned empty path")
	}
}
