module github.com/alexbacchin/ssm-session-client

go 1.24.0

require (
	github.com/aws/aws-sdk-go-v2 v1.41.1
	github.com/aws/aws-sdk-go-v2/config v1.32.7
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.286.0
	github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect v1.32.16
	github.com/aws/aws-sdk-go-v2/service/ssm v1.67.8
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.6
	github.com/aws/session-manager-plugin v0.0.0-20251119232423-83812b6d1d99
	github.com/aws/smithy-go v1.24.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.21.0
	golang.org/x/net v0.50.0
	golang.org/x/sys v0.41.0
)

require (
	github.com/aws/aws-sdk-go-v2/credentials v1.19.7
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.9
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.13
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	go.uber.org/zap v1.27.1
	gopkg.in/ini.v1 v1.67.1
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.5 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/eiannone/keyboard v0.0.0-20220611211555-0d226195f203 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/twinj/uuid v1.0.0 // indirect
	github.com/xtaci/smux v1.5.56 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/term v0.40.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/twinj/uuid => github.com/twinj/uuid v0.0.0-20151029044442-89173bcdda19 // indirect
