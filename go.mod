module github.com/mmmorris1975/ssm-session-client

go 1.25.0

require (
	github.com/aws/aws-sdk-go-v2 v1.41.7
	github.com/aws/aws-sdk-go-v2/config v1.32.17
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.303.0
	github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect v1.32.22
	github.com/aws/aws-sdk-go-v2/service/ssm v1.68.6
	github.com/aws/session-manager-plugin v0.0.0-20260317185859-1fc63e3f5a01
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.4.2
	golang.org/x/net v0.54.0
	golang.org/x/sys v0.45.0
)

require (
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.16 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.23 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.1 // indirect
	github.com/aws/smithy-go v1.25.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/twinj/uuid v0.0.0-20151029044442-89173bcdda19 // indirect
	github.com/xtaci/smux v1.5.35 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/term v0.43.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// REF: https://github.com/aws/session-manager-plugin/issues/1
// replace github.com/aws/SSMCLI => github.com/aws/session-manager-plugin v0.0.0-20250205214155-b2b0bcd769d1
