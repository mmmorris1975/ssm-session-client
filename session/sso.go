//Source: github.com/peterHoburg/aws-sdk-go-v2-sso-login

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/pkg/browser"
	"go.uber.org/zap"
	"gopkg.in/ini.v1"
)

type SSOLoginInput struct {
	// ProfileName name of the profile in ~/.aws/config. [profile <ProfileName>]
	ProfileName string

	// LoginTimeout max time to wait for user to complete the SSO OIDC URL flow. This should be > 60 seconds. Default value is 90 seconds
	LoginTimeout time.Duration

	// Headed if true a browser will be opened with the URL for the SSO OIDC flow. You will have the [LoginTimeout] to
	// complete the flow in the browser.
	Headed bool

	// ForceLogin if true forces a new SSO OIDC flow even if the cached creds are still valid.
	ForceLogin bool
}

func (v *SSOLoginInput) validate() error {
	if v.LoginTimeout == 0 {
		v.LoginTimeout = 90 * time.Second
	}
	return nil
}

// IdentityResult contains the result of stsClient.GetCallerIdentity. If Identity is nul and error is not nul that
// can indicate that the credentials might be invalid.
type IdentityResult struct {
	Identity *sts.GetCallerIdentityOutput
	Error    error
}

type SSOLoginOutput struct {
	Credentials      *aws.Credentials
	CredentialsCache *aws.CredentialsCache
	IdentityResult   *IdentityResult
}

type configProfile struct {
	name         string
	output       string
	region       string
	ssoAccountId string
	ssoRegion    string
	ssoRoleName  string
	ssoStartUrl  string
	ssoSession   string
}

func (v *configProfile) validate(profileName string, configFilePath string) error {
	if v.name == "" {
		return NewProfileValidationError(profileName, configFilePath, "name", v.name, "<non empty>")
	}
	if v.output == "" {
		v.output = "json"
	}
	if v.region == "" {
		return NewProfileValidationError(profileName, configFilePath, "region", v.region, "<non empty>")
	}
	if v.ssoAccountId == "" {
		return NewProfileValidationError(profileName, configFilePath, "sso_account_id", v.ssoAccountId, "<non empty>")
	}
	if v.ssoRegion == "" {
		return NewProfileValidationError(profileName, configFilePath, "sso_region", v.ssoRegion, "<non empty>")
	}
	if v.ssoRoleName == "" {
		return NewProfileValidationError(profileName, configFilePath, "sso_role_name", v.ssoRoleName, "<non empty>")
	}
	if v.ssoStartUrl == "" {
		return NewProfileValidationError(profileName, configFilePath, "sso_start_url", v.ssoStartUrl, "<non empty>")
	}
	return nil
}

type cacheFileData struct {
	StartUrl              string    `json:"startUrl"`
	Region                string    `json:"region"`
	AccessToken           string    `json:"accessToken"`
	ExpiresAt             time.Time `json:"expiresAt"`
	ClientId              string    `json:"clientId"`
	ClientSecret          string    `json:"clientSecret"`
	RegistrationExpiresAt time.Time `json:"registrationExpiresAt"`
}

// Login runs through the AWS CLI login flow if there isn't a ~/.aws/sso/cache file with valid creds. If ForceLogin is
// true then the login flow will always be triggered even if the cache is valid
func SSOLogin(ctx context.Context, params *SSOLoginInput) (*SSOLoginOutput, error) {
	var creds *aws.Credentials
	var credCache *aws.CredentialsCache
	var credCacheError error

	err := params.validate()
	if err != nil {
		zap.S().Error("Error validating SSOLoginInput: ", err)
		return nil, err
	}
	zap.S().Debug("SSO LoginInput: ", params)
	configFilePath := config.DefaultSharedConfigFilename()
	zap.S().Debug("SSO Config file path: ", configFilePath)
	profile, err := getConfigProfile(params.ProfileName, configFilePath)
	if err != nil {
		zap.S().Error("SSO Error getting config profile: ", err)
		return nil, err
	}
	zap.S().Debug("Config profile: ", profile)
	cacheFilePath, err := getCacheFilePath(profile)
	if err != nil {
		zap.S().Error("SSO Error getting cache file path: ", err)
		return nil, err
	}
	zap.S().Debug("Cache file path: ", cacheFilePath)

	// This does not need to be run if ForceLogin is set, but doing it simplifies the overall flow, and is still fast.
	creds, credCache, credCacheError = getAwsCredsFromCache(ctx, profile, cacheFilePath)
	identity, callerIDError := getCallerID(ctx)

	// Creds are invalid, try logging in again
	if credCacheError != nil || callerIDError != nil || params.ForceLogin {
		cacheFile, err := ssoLoginFlow(ctx, profile, params.Headed, params.LoginTimeout)
		if err != nil {
			zap.S().Error("Error running ssoLoginFlow: ", err)
			return nil, err
		}

		err = writeCacheFile(cacheFile, cacheFilePath)
		if err != nil {
			zap.S().Error("Error writing cache file: ", err)
			return nil, err
		}

		creds, credCache, credCacheError = getAwsCredsFromCache(ctx, profile, cacheFilePath)
		if credCacheError != nil {
			zap.S().Error("Error getting creds from cache: ", credCacheError)
			return nil, credCacheError
		}

		identity, callerIDError = getCallerID(ctx)
	}

	loginOutput := &SSOLoginOutput{
		Credentials:      creds,
		CredentialsCache: credCache,
		IdentityResult: &IdentityResult{
			Identity: identity,
			Error:    callerIDError,
		},
	}
	zap.S().Debug("SSOLoginOutput: ", loginOutput)

	return loginOutput, nil
}

func getCacheFilePath(profile *configProfile) (string, error) {
	var cacheFilePath string
	var err error

	if profile.ssoSession != "" {
		cacheFilePath, err = ssocreds.StandardCachedTokenFilepath(profile.ssoSession)

	} else {
		cacheFilePath, err = ssocreds.StandardCachedTokenFilepath(profile.ssoStartUrl)
	}

	if err != nil {
		return "", NewCacheFilepathGenerationError(profile.name, profile.ssoStartUrl, err)
	}
	return cacheFilePath, nil
}

// writeCacheFile Writes the cache file that is read by the AWS CLI.
func writeCacheFile(cacheFileData *cacheFileData, cacheFilePath string) error {
	marshaledJson, err := json.Marshal(cacheFileData)
	if err != nil {
		return CacheFileCreationError{err, "failed to marshal json", cacheFilePath}
	}
	dir, _ := filepath.Split(cacheFilePath)
	zap.S().Debug("Cache file dir: ", dir)
	err = os.MkdirAll(dir, 0600)
	if err != nil {
		return CacheFileCreationError{err, "failed to create directory", cacheFilePath}
	}

	err = os.WriteFile(cacheFilePath, marshaledJson, 0600)
	if err != nil {
		return CacheFileCreationError{err, "failed to write file", cacheFilePath}

	}
	return nil
}

// findIniSection parses ini file that has sections in [type name] format.
func findIniSection(iniFile *ini.File, sectionType string, sectionName string) *ini.Section {
	for _, section := range iniFile.Sections() {
		fullSectionName := strings.TrimSpace(section.Name())

		if !strings.HasPrefix(strings.ToLower(fullSectionName), sectionType) {
			continue
		}
		trimmedProfileName := strings.TrimSpace(strings.TrimPrefix(fullSectionName, sectionType))
		if trimmedProfileName != sectionName {
			continue
		}
		return section
	}
	return nil
}

// setDefaults I tried doing this dynamically with reflection and some fun camelCase shenanigans,
// but configProfile fields would need to be public, and I don't care that much.
func setDefaults(profile *configProfile, defaultSection *ini.Section) {
	if defaultSection == nil {
		return
	}
	if profile.region == "" {
		profile.region = defaultSection.Key("region").String()
	}
	if profile.output == "" {
		profile.output = defaultSection.Key("output").String()
	}
}

func getConfigProfile(profileName string, configFilePath string) (*configProfile, error) {
	// You have to set IgnoreInlineComment: true because ...start#/ is common in the sso_start_url
	// gopkg.in/ini.v1@v1.67.0/parser.go:281 will remove everything after #
	configFile, err := ini.LoadSources(ini.LoadOptions{IgnoreInlineComment: true}, configFilePath)
	if err != nil {
		return nil, NewLoadingConfigFileError(configFilePath, err)
	}

	section := findIniSection(configFile, "profile", profileName)
	if section == nil {
		return nil, NewMissingProfileError(profileName, configFilePath, err)
	}

	profile := configProfile{
		name:         profileName,
		output:       section.Key("output").Value(),
		region:       section.Key("region").Value(),
		ssoAccountId: section.Key("sso_account_id").Value(),
		ssoRegion:    section.Key("sso_region").Value(),
		ssoRoleName:  section.Key("sso_role_name").Value(),
		ssoStartUrl:  section.Key("sso_start_url").Value(),
	}
	ssoSession := section.Key("sso_session").Value()
	if ssoSession != "" {
		profile.ssoSession = ssoSession
		ssoSessionData := findIniSection(configFile, "sso-session", ssoSession)
		if ssoSessionData != nil {
			profile.ssoRegion = ssoSessionData.Key("sso_region").Value()
			profile.ssoStartUrl = ssoSessionData.Key("sso_start_url").Value()
		}
	}

	defaultSection := findIniSection(configFile, "default", "")
	setDefaults(&profile, defaultSection)

	err = profile.validate(profileName, configFilePath)
	if err != nil {
		return nil, err
	}

	return &profile, nil
}

// getAwsCredsFromCache
func getAwsCredsFromCache(
	ctx context.Context,
	profile *configProfile,
	cacheFilePath string,
) (*aws.Credentials, *aws.CredentialsCache, error) {
	cfg, err := BuildAWSConfig(ctx, "default")
	if err != nil {
		return nil, nil, err
	}
	ssoClient := sso.NewFromConfig(cfg)
	ssoOidcClient := ssooidc.NewFromConfig(cfg)

	ssoCredsProvider := ssocreds.New(
		ssoClient,
		profile.ssoAccountId,
		profile.ssoRoleName,
		profile.ssoStartUrl,
		func(options *ssocreds.Options) {
			options.SSOTokenProvider = ssocreds.NewSSOTokenProvider(ssoOidcClient, cacheFilePath)
		},
	)

	credCache := aws.NewCredentialsCache(ssoCredsProvider)
	creds, err := credCache.Retrieve(ctx)
	if err != nil {
		return nil, nil, CredCacheError{err}
	}
	return &creds, credCache, nil
}

func ssoLoginFlow(
	ctx context.Context,
	profile *configProfile,
	headed bool,
	loginTimeout time.Duration,
) (*cacheFileData, error) {
	// RegisterClient, StartDeviceAuthorization, and CreateToken are all unsigned
	// SSO OIDC operations — they require only the SSO region, not AWS credentials.
	// Using BuildAWSConfig here would attempt to resolve credentials for the
	// profile (e.g. assume-role chains), which fails before any login can occur.
	ssoOidcClient := ssooidc.NewFromConfig(aws.Config{
		Region:     profile.ssoRegion,
		HTTPClient: ProxyHttpClient(),
	})

	currentUser, err := user.Current()
	if err != nil {
		return nil, OsUserError{err}
	}

	clientName := fmt.Sprintf("%s-%s-%s", currentUser.Username, profile.name, profile.ssoRoleName)
	registerClient, err := ssoOidcClient.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String(clientName),
		ClientType: aws.String("public"),
		Scopes:     []string{"sso-portal:*"},
	})
	if err != nil {
		return nil, SsoOidcClientError{err}
	}

	deviceAuth, err := ssoOidcClient.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     registerClient.ClientId,
		ClientSecret: registerClient.ClientSecret,
		StartUrl:     &profile.ssoStartUrl,
	})
	if err != nil {
		return nil, StartDeviceAuthorizationError{err}
	}

	authUrl := aws.ToString(deviceAuth.VerificationUriComplete)
	if headed {
		fmt.Fprintf(os.Stderr, "Please approve the authorization request with user code: %s\n", *deviceAuth.UserCode)
		err = browser.OpenURL(authUrl)
		if err != nil {
			return nil, BrowserOpenError{err}
		}
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Open the following URL in your browser: %s\n", authUrl)
	}

	var createTokenErr error
	token := new(ssooidc.CreateTokenOutput)
	sleepPerCycle := 2 * time.Second
	startTime := time.Now()
	delta := time.Since(startTime)

	for delta < loginTimeout {
		// Keep trying until the user approves the request in the browser
		token, createTokenErr = ssoOidcClient.CreateToken(
			ctx, &ssooidc.CreateTokenInput{
				ClientId:     registerClient.ClientId,
				ClientSecret: registerClient.ClientSecret,
				DeviceCode:   deviceAuth.DeviceCode,
				GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
			},
		)
		if createTokenErr == nil {
			break
		}
		if strings.Contains(createTokenErr.Error(), "AuthorizationPendingException") {
			time.Sleep(sleepPerCycle)
			delta = time.Since(startTime)
			continue
		}
	}
	// Checks to see if there is a valid token after the login timeout ends
	if createTokenErr != nil || token.AccessToken == nil {
		return nil, SsoOidcTokenCreationError{err}
	}
	cacheFile := cacheFileData{
		StartUrl:              profile.ssoStartUrl,
		Region:                profile.region,
		AccessToken:           *token.AccessToken,
		ExpiresAt:             time.Unix(time.Now().Unix()+int64(token.ExpiresIn), 0).UTC(),
		ClientSecret:          *registerClient.ClientSecret,
		ClientId:              *registerClient.ClientId,
		RegistrationExpiresAt: time.Unix(registerClient.ClientSecretExpiresAt, 0).UTC(),
	}

	return &cacheFile, nil
}

func getCallerID(ctx context.Context) (*sts.GetCallerIdentityOutput, error) {
	stsConfig, err := BuildAWSConfig(ctx, "sts")
	if err != nil {
		return nil, err
	}
	stsClient := sts.NewFromConfig(stsConfig)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, GetCallerIdError{err}
	}
	return identity, nil
}

// ProfileValidationError error validating the given AWS profile. A required value may be missing.
type ProfileValidationError struct {
	ProfileName    string
	ConfigFilePath string
	FieldName      string
	CurrentValue   string
	ExpectedValue  string
}

func (e ProfileValidationError) Error() string {

	return fmt.Sprintf(
		"Profile validation failed. "+
			"Profile: %s "+
			"Config file path: %s "+
			"Field %s "+
			"Value: \"%s\" "+
			"Expected \"%s\"",
		e.ProfileName,
		e.ConfigFilePath,
		e.FieldName,
		e.CurrentValue,
		e.ExpectedValue,
	)
}

func NewProfileValidationError(profileName string, configFilePath string, fieldName string, currentValue string, expectedValue string) ProfileValidationError {
	return ProfileValidationError{profileName, configFilePath, fieldName, currentValue, expectedValue}
}

// LoadingConfigFileError failed to load the config file
type LoadingConfigFileError struct {
	ConfigFilePath string
	Err            error
}

func NewLoadingConfigFileError(configFilePath string, err error) LoadingConfigFileError {
	return LoadingConfigFileError{configFilePath, err}
}

func (e LoadingConfigFileError) Error() string {
	return fmt.Sprintf("Failed to load config file: %s", e.ConfigFilePath)
}

func (e LoadingConfigFileError) Unwrap() error {
	return e.Err
}

// MissingProfileError failed to find the requested profile
type MissingProfileError struct {
	ProfileName    string
	ConfigFilePath string
	Err            error
}

func NewMissingProfileError(profileName string, configFilePath string, err error) MissingProfileError {
	return MissingProfileError{profileName, configFilePath, err}
}

func (e MissingProfileError) Error() string {
	return fmt.Sprintf("Profile %s does not exist in config file %s", e.ProfileName, e.ConfigFilePath)
}

func (e MissingProfileError) Unwrap() error {
	return e.Err
}

// CacheFilepathGenerationError failed to generate a valid filepath for the given SSO start URL
type CacheFilepathGenerationError struct {
	ProfileName        string
	ProfileSSOStartURL string
	Err                error
}

func NewCacheFilepathGenerationError(ProfileName string, ProfileSSOStartURL string, err error) CacheFilepathGenerationError {
	return CacheFilepathGenerationError{ProfileName, ProfileSSOStartURL, err}
}

func (e CacheFilepathGenerationError) Error() string {
	return fmt.Sprintf(
		"Failed to generate cache file path for profile '%s' with URL %s",
		e.ProfileName,
		e.ProfileSSOStartURL,
	)
}

func (e CacheFilepathGenerationError) Unwrap() error {
	return e.Err
}

// ConfigFileLoadError failed to load default config
type ConfigFileLoadError struct {
	Err error
}

func (e ConfigFileLoadError) Error() string {
	return "failed to load default config"
}

func (e ConfigFileLoadError) Unwrap() error {
	return e.Err
}

// CredCacheError failed to retrieve creds from ssoCredsProvider
type CredCacheError struct {
	Err error
}

func (e CredCacheError) Error() string {
	return "failed to retrieve creds from ssoCredsProvider"
}

func (e CredCacheError) Unwrap() error {
	return e.Err
}

// OsUserError failed to retrieve user from osUser
type OsUserError struct {
	Err error
}

func (e OsUserError) Error() string {
	return "failed to retrieve user from osUser"
}

func (e OsUserError) Unwrap() error {
	return e.Err
}

// SsoOidcClientError Failed to register ssoOidcClient
type SsoOidcClientError struct {
	Err error
}

func (e SsoOidcClientError) Error() string {
	return "Failed to register ssoOidcClient"
}

func (e SsoOidcClientError) Unwrap() error {
	return e.Err
}

// StartDeviceAuthorizationError Failed to startDeviceAuthorization
type StartDeviceAuthorizationError struct {
	Err error
}

func (e StartDeviceAuthorizationError) Error() string {
	return "Failed to startDeviceAuthorization"
}

func (e StartDeviceAuthorizationError) Unwrap() error {
	return e.Err
}

// BrowserOpenError Failed to open a browser
type BrowserOpenError struct {
	Err error
}

func (e BrowserOpenError) Error() string {
	return "Failed to open a browser"
}

func (e BrowserOpenError) Unwrap() error {
	return e.Err
}

// SsoOidcTokenCreationError failed to retrieve user from osUser
type SsoOidcTokenCreationError struct {
	Err error
}

func (e SsoOidcTokenCreationError) Error() string {
	return "failed to retrieve user from osUser"
}

func (e SsoOidcTokenCreationError) Unwrap() error {
	return e.Err
}

// GetCallerIdError stsClient.GetCallerIdentity failed
type GetCallerIdError struct {
	Err error
}

func (e GetCallerIdError) Error() string {
	return "stsClient.GetCallerIdentity failed"
}

func (e GetCallerIdError) Unwrap() error {
	return e.Err
}

type CacheFileCreationError struct {
	Err           error
	Reason        string
	CacheFilePath string
}

func (e CacheFileCreationError) Error() string {
	return fmt.Sprintf("Cache file %s creation failed. Reason: %s", e.CacheFilePath, e.Reason)
}

func (e CacheFileCreationError) Unwrap() error {
	return e.Err
}
