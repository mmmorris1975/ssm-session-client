module github.com/mmmorris1975/ssm-session-client

go 1.15

require (
	github.com/aws/aws-sdk-go v1.44.76 // indirect
	github.com/aws/aws-sdk-go-v2 v1.17.1
	github.com/aws/aws-sdk-go-v2/config v1.17.10
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.64.0
	github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect v1.14.11
	github.com/aws/aws-sdk-go-v2/service/ssm v1.31.3
	github.com/aws/session-manager-plugin v0.0.0-20221012155945-c523002ee02c
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/eiannone/keyboard v0.0.0-20220611211555-0d226195f203 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/google/uuid v1.3.0
	github.com/gorilla/websocket v1.4.2
	github.com/stretchr/testify v1.8.0 // indirect
	github.com/twinj/uuid v0.0.0-20151029044442-89173bcdda19 // indirect
	github.com/xtaci/smux v1.5.16 // indirect
	golang.org/x/crypto v0.0.0-20220722155217-630584e8d5aa // indirect
	golang.org/x/net v0.0.0-20220812174116-3211cb980234
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4 // indirect
	golang.org/x/sys v0.0.0-20220811171246-fbc7d0a398ab
)

// REF: https://github.com/aws/session-manager-plugin/issues/1
// replace github.com/aws/SSMCLI => github.com/aws/session-manager-plugin v0.0.0-20221012155945-c523002ee02c
