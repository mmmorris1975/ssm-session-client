module github.com/mmmorris1975/ssm-session-client

go 1.26.0

require (
	github.com/aws/aws-sdk-go-v2 v1.47.3
	github.com/aws/aws-sdk-go-v2/config v1.33.9
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.339.0
	github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect v1.44.1
	github.com/aws/aws-sdk-go-v2/service/ssm v1.79.3
	github.com/aws/session-manager-plugin v0.0.0-20260615221425-930a08e65d3a
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.4.2
	golang.org/x/net v0.61.0
	golang.org/x/sys v0.49.0
)

require (
	github.com/aws/aws-sdk-go-v2/credentials v1.20.9 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.61.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.10.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.38.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.43.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.51.4 // indirect
	github.com/aws/smithy-go v1.28.5 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/twinj/uuid v0.0.0-20151029044442-89173bcdda19 // indirect
	github.com/xtaci/smux v1.5.35 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/term v0.47.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// REF: https://github.com/aws/session-manager-plugin/issues/1
// replace github.com/aws/SSMCLI => github.com/aws/session-manager-plugin v0.0.0-20250205214155-b2b0bcd769d1
