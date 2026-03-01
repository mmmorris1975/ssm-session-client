# SSM Session Client CLI

This project is a fork of [ssm-session-client](https://github.com/mmmorris1975/ssm-session-client) with added CLI functionality to connect to AWS SSM sessions. The goal is to provide a single executable for SSM Session functionality, especially useful in environments where AWS CLI execution is restricted. Such as:

- Microsoft AppLocker
- AirLock
- Manage Engine

The main goal of this project is to enable SSM Client in complex environments where AWS Services endpoints (PrivateLink) are accessible from private networks via VPN or Direct Connect.

When the SSM `StartSession` is called, the API will always return the [StreamUrl](https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_StartSession.html#API_StartSession_ResponseSyntax) with the regional SSM Messages endpoint. Even if when a SSM Messages endpoint PrivateLink is reachable in a private network, the only options to use it for session streams are HTTPS proxy or [DNS RPZ](https://dnsrpz.info/). For this reason, this app has a flag to set the SSM Messages endpoint, then it will replace the StreamUrl with your SSM Messages endpoint.

**Note**: [Windows SSH Client](https://learn.microsoft.com/en-us/windows/terminal/tutorials/ssh#access-windows-ssh-client-and-ssh-server) is not installed by default. The `ssh-direct` command and VSCode integration eliminate the need for an external SSH client entirely.

## Requirements

1. Download the executable from [Releases](https://github.com/alexbacchin/ssm-session-client/releases) and copy it to the target operating system.
2. (Optional) Install the [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html). It is recommended and will be used by default if installed.

## Configuration

First, follow the standard [configure AWS SDK and Tools](https://docs.aws.amazon.com/sdkref/latest/guide/creds-config-files.html) to provide AWS credentials.

The utility can be configured via:

1. Configuration file: Default is `$HOME/.ssm-session-client.yaml`. It will also search in:
   a. Current folder
   b. User's home folder
   c. Application folder
2. Environment Variables with the `SSC_` prefix
3. Command Line parameters

These are the configuration options:

| Description                          | App Config/Flag       | App Env Variable         | AWS SDK Variable                |
| :----------------------------------: | :-------------------: | :----------------------: | :------------------------------:|
| Config File Path                     | config                | SSC_CONFIG               | n/a                             |
| Log Level                            | log-level             | SSC_LOG_LEVEL            | n/a                             |
| AWS SDK profile name                 | aws-profile           | SSC_AWS_PROFILE          | AWS_PROFILE                     |
| AWS SDK region name                  | aws-region            | SSC_AWS_REGION           | AWS_REGION or AWS_DEFAULT_REGION|
| AWS SDK SSO Login (true/false)       | sso-login             | SSC_SSO_LOGIN            | n/a                             |
| AWS SDK SSO Open Browser (true/false)| sso-open-browser      | SSC_SSO_OPEN_BROWSER     | n/a                             |
| STS Endpoint                         | sts-endpoint          | SSC_STS_ENDPOINT         | AWS_ENDPOINT_URL_STS            |
| EC2 Endpoint                         | ec2-endpoint          | SSC_EC2_ENDPOINT         | AWS_ENDPOINT_URL_EC2            |
| SSM Endpoint                         | ssm-endpoint          | SSC_SSM_ENDPOINT         | AWS_ENDPOINT_URL_SSM            |
| SSM Messages Endpoint                | ssmmessages-endpoint  | SSC_SSMMESSAGES_ENDPOINT | n/a                             |
| Proxy URL                            | proxy-url             | SSC_PROXY_URL            | HTTPS_PROXY                     |
| SSM Session Plugin (true/false)      | ssm-session-plugin    | SSC_SSM_SESSION_PLUGIN   | n/a                             |
| Auto Reconnect (true/false)          | enable-reconnect      | SSC_ENABLE_RECONNECT     | n/a                             |
| Max Reconnection Attempts            | max-reconnects        | SSC_MAX_RECONNECTS       | n/a                             |
| Target Aliases (config file only)    | aliases               | n/a                      | n/a                             |
| Target Alias (CLI flag, repeatable)  | --alias name=tag:val  | n/a                      | n/a                             |

### Remarks

- The `proxy-url` flag is only applicable to services where custom endpoints are not set.
- The `ssmmessages-endpoint` flag is used to perform the WSS connection during an SSM Session by replacing the StreamUrl with the SSM Messages endpoint.

### Logging

Logging is generated on the console and log file at:

- Windows: `%USERPROFILE%\AppData\Local\ssm-session-client\logs`
- MACOS: `$HOME/Library/Logs/ssm-session-client`
- Linux and other Unix-like systems: `$HOME/.ssm-session-client/logs`

Log files are rotated daily or when size reaches 10MB and the last 3 log files are kept.

### AWS Credentials

This utility will use AWS SDK credentials and profiles. More info [Authentication and access credentials for the AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-authentication.html)

### AWS Identity Center SSO Login

If you have AWS SSO via Identity Center deployed, the `ssm-session-client` can automatically open the browser and perform device code authentication. This enhances the experience as the authentication to AWS happens in the browser. In case your operating system does not have a browser, you can copy and paste the URL to another browser instead. Here are the steps to get SSO configured

1. [Configure AWS CLI SSO via Identity Center](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html)
2. Modify the configuration file (or environment variables) with `aws-profile` pointing to the AWS profile name with SSO, and `sso-login` set to `true`
3. (Optional) If your operating system can open a browser, also set `sso-open-browser` to `true`.

#### Sample config file

```yaml
ec2-endpoint: vpce-059c3b85db66f8165-mzb6o9nb.ec2.us-west-2.vpce.amazonaws.com
ssm-endpoint: vpce-06ef6f173680a1306-bt58rzff.ssm.us-west-2.vpce.amazonaws.com
ssmmessages-endpoint: vpce-0e5e5b0c558a14bf2-r3p6zkdm.ssmmessages.us-west-2.vpce.amazonaws.com
sts-endpoint: vpce-0877b4abeb479ee06-arkdktlc.sts.us-west-2.vpce.amazonaws.com
aws-profile: sandbox
proxy-url: http://myproxy:3128
log-level: warn
sso-login: false
```

#### Sample config file with SSO

```yaml
aws-profile: sandbox-sso
sso-login: true
sso-open-browser: true
aws-region: ap-southeast-2
```

## Supported Modes

### Shell

Shell-level access to an instance can be obtained using the `shell` command. This command requires an AWS SDK profile and a string to identify the target instance.

**Note**: If you have enabled KMS encryption for Sessions, you must use the AWS Session Manager plugin.

```shell
$ssm-session-client shell i-0bdb4f892de4bb54c --config=config.yaml
```

On Windows, the shell command supports virtual terminal processing with ANSI escape sequences for proper color and formatting in remote shells.

IAM: [Sample IAM policies for Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/getting-started-restrict-access-quickstart.html)

### SSH (ProxyCommand Mode)

SSH over SSM integration can be used via the `ssh` command. Ensure the target instance has SSH authentication configured before connecting. This feature is meant to be used in SSH configuration files according to the [AWS documentation](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html).

You need to configure the ProxyCommand in `$HOME/.ssh/config` (Linux/macOS) or `%USERPROFILE%\.ssh\config` (Windows).

```shell
# SSH over Session Manager
Host i-*
  ProxyCommand ssm-session-client ssh %r@%h --ssm-session-plugin=true --config=config.yaml
```

You can also automatically configure the ProxyCommand using the `add-proxy-command` subcommand:

```shell
# Automatically add/update SSH config entry
$ssm-session-client ssh add-proxy-command ec2-user@i-0bdb4f892de4bb54c
```

Then to connect:

```shell
$ssh ec2-user@i-0bdb4f892de4bb54c
```

IAM: [Controlling user permissions for SSH connections through Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html)

### SSH Direct (Native SSH Client, No External Dependencies)

The `ssh-direct` command provides a fully integrated SSH experience over SSM without requiring an external SSH client. It uses Go's native SSH library (`golang.org/x/crypto/ssh`) internally.

```shell
# Interactive SSH session
$ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0

# With a specific SSH key
$ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --ssh-key ~/.ssh/my-key

# Execute a command
$ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --exec "uptime"

# Using EC2 Instance Connect (no key management needed)
$ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --instance-connect

# Custom port
$ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0:2222
```

**Authentication chain** (tried in order):
1. Ephemeral key (when `--instance-connect` is used)
2. SSH agent (`SSH_AUTH_SOCK`)
3. Private key file (`--ssh-key` or auto-discovered `~/.ssh/id_ed25519`, `~/.ssh/id_rsa`)
4. Password prompt (interactive)

**Host key verification** uses Trust-On-First-Use (TOFU) with `~/.ssh/known_hosts` integration. Use `--no-host-key-check` to skip verification.

#### VSCode Remote SSH Integration

`ssm-session-client` can replace the SSH executable entirely for VSCode Remote SSH, eliminating the need to install an SSH client or configure ProxyCommand. This is particularly useful on Windows where OpenSSH may not be installed.

**Step 1: Configure VSCode**

In VSCode settings (`settings.json`), set the path to the `ssm-session-client` binary:

```json
{
  "remote.SSH.path": "/path/to/ssm-session-client"
}
```

On Windows:
```json
{
  "remote.SSH.path": "C:\\path\\to\\ssm-session-client.exe"
}
```

**Step 2: Configure SSH Config**

Create or edit your SSH config file (`~/.ssh/config` on macOS/Linux, `%USERPROFILE%\.ssh\config` on Windows):

```
Host my-ec2-instance
  HostName i-0123456789abcdef0
  User ec2-user

# Wildcard for all instance IDs
Host i-*
  User ec2-user
  StrictHostKeyChecking accept-new
```

The `HostName` directive maps the friendly name to the EC2 instance ID. You can also use instance IDs directly as the host name.

**Step 3: Configure AWS Credentials**

Ensure AWS credentials are available via environment variables (`AWS_PROFILE`, `AWS_REGION`) or the standard AWS SDK credential chain. You can also use the `ssm-session-client` configuration file (`~/.ssm-session-client.yaml`) for VPC endpoints and other settings.

**Step 4: Connect**

In VSCode, use "Remote-SSH: Connect to Host..." and select your configured host.

**How It Works**

When VSCode invokes `ssm-session-client` with OpenSSH-compatible flags (e.g., `-T -o ConnectTimeout=15 -F /path/to/config hostname bash`), it automatically detects SSH-compat mode:
1. Parses OpenSSH-style arguments (`-T`, `-o`, `-F`, `-p`, `-l`, `-i`, etc.)
2. Reads the SSH config file (`-F` or `~/.ssh/config`)
3. Resolves the hostname to an EC2 instance ID via the `HostName` directive
4. Establishes an SSH-over-SSM tunnel using the native Go SSH client

**Supported SSH Flags**

| Flag | Description |
|------|-------------|
| `-T` | Disable PTY allocation (used by VSCode) |
| `-t` | Force PTY allocation |
| `-N` | No remote command |
| `-o key=value` | SSH options (StrictHostKeyChecking, UserKnownHostsFile, ConnectTimeout, etc.) |
| `-F <config>` | SSH config file path |
| `-p <port>` | Remote port |
| `-l <user>` | Login username |
| `-i <keyfile>` | Identity file (private key) |
| `-D <port>` | Dynamic port forwarding (accepted but ignored) |
| `-v/-vv/-vvv` | Verbosity (maps to log levels) |

**Symlink Mode**

Alternatively, you can create a symlink named `ssh` pointing to `ssm-session-client`:

```shell
ln -s /path/to/ssm-session-client /usr/local/bin/ssh
```

When invoked as `ssh`, it always operates in OpenSSH-compat mode.

### SSH with Instance Connect (Linux targets only)

SSH over SSM with [EC2 Instance Connect](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/connect-linux-inst-eic.html) can be used via the `instance-connect` command. This configuration is similar to the SSH setup above, but SSH authentication configuration is not required. Authentication is managed by the IAM action `ec2-instance-connect:SendSSHPublicKey`.

In this mode, the app will attempt to use default public SSH keys (`id_ed25519.pub` and `id_rsa.pub`) for temporary SSH authentication. Alternatively, you can specify a custom public key file using the `ssh-public-key-file` flag.

**Note**: EC2 Instance Connect endpoints are not available via AWS PrivateLink. Internet access or an Internet proxy is required to use this mode.

```shell
# SSH over Session Manager with EC2 Instance Connect and default SSH keys
Host i-*
  ProxyCommand ssm-session-client instance-connect %r@%h --ssm-session-plugin=true --config=config.yaml
```

```shell
# SSH over Session Manager with EC2 Instance Connect and custom SSH keys
Host i-*
  IdentityFile ~/.ssh/custom
  ProxyCommand ssm-session-client instance-connect %r@%h --ssm-session-plugin=true --config=config.yaml --ssh-public-key-file=~/.ssh/custom.pub
```

You can also automatically configure the ProxyCommand:

```shell
$ssm-session-client instance-connect add-proxy-command ec2-user@i-0bdb4f892de4bb54c
```

Then to connect:

```shell
$ssh ec2-user@i-0bdb4f892de4bb54c
```

IAM:

- [Controlling user permissions for SSH connections through Session Manager](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html)
- [Grant IAM permissions for EC2 Instance Connect](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-connect-configure-IAM-role.html)

### Port Forwarding

Port Forwarding via SSM allows you to securely create tunnels between your instances deployed in private subnets without needing to start the SSH service on the server, open the SSH port in the security group, or use a bastion host. It can be used via the `port-forwarding` command. If a local port is not provided, a random local port will be assigned.

```shell
# Port forwarding from local port 8888 to instance port 443
$ssm-session-client port-forwarding i-0bdb4f892de4bb54c:443 8888 --config=config.yaml
```

**Multiplexed connections**: For SSM agents v3.0.196.0 and above, port forwarding automatically uses stream multiplexing, allowing multiple concurrent TCP connections over a single SSM session. Older agents fall back to single-connection mode.

### RDP (Windows only)

The `rdp` command provides a streamlined way to connect to Windows EC2 instances via Remote Desktop Protocol through SSM. It sets up a port-forwarding tunnel and launches the Windows RDP client (`mstsc.exe`) automatically.

```shell
# Basic RDP connection
$ssm-session-client rdp i-0bdb4f892de4bb54c

# RDP with automatic password retrieval
$ssm-session-client rdp i-0bdb4f892de4bb54c --get-password --key-pair-file ~/.ssh/my-ec2-keypair.pem

# Custom RDP port and username
$ssm-session-client rdp i-0bdb4f892de4bb54c --rdp-port 3390 --username admin

# Specific local port
$ssm-session-client rdp i-0bdb4f892de4bb54c --local-port 33389
```

| Flag | Default | Description |
|------|---------|-------------|
| `--rdp-port` | 3389 | Remote RDP port on the EC2 instance |
| `--local-port` | 0 (auto) | Local port for the SSM tunnel |
| `--get-password` | false | Retrieve Windows administrator password via AWS API |
| `--key-pair-file` | | Path to EC2 key pair private key (required with `--get-password`) |
| `--username` | Administrator | RDP username |

When `--get-password` is used, the password is retrieved from the EC2 API, decrypted using the provided key pair, and copied to the clipboard so it can be pasted on the RDP client credentials prompt.

IAM: Requires `ec2:GetPasswordData` permission in addition to SSM session permissions.

## Target Lookup

The target can be an instance ID, hostname, IP address, or a named alias. The app resolves the target using the following chain (first match wins):

| Priority | Method | Example input | How it works |
|----------|--------|---------------|--------------|
| 1 | **Instance ID** | `i-0abc1234def56789` | Passed through directly |
| 2 | **Alias** | `devbox` | Looks up the name in the configured alias map and queries EC2 by tag |
| 3 | **Tag** | `Name:web-server` | Queries EC2 for a running instance with the given tag key:value |
| 4 | **IP address** | `10.0.1.5` | Queries EC2 by private or public IPv4 address |
| 5 | **DNS TXT record** | `web.example.com` | Looks up a DNS TXT record expected to contain the instance ID |

### Target Aliases

Aliases map a short name to an EC2 tag key and value. They are useful when you want to refer to an instance by a friendly name without typing the full `tag-name:tag-value` format every time. Unlike the generic tag resolver, an alias lookup **errors if more than one running instance matches**, ensuring the alias always refers to a unique host.

#### Defining aliases in the config file

```yaml
aliases:
  devbox:
    tag-name: Name
    tag-value: my-devbox
  prod-web:
    tag-name: Environment
    tag-value: production
```

#### Defining aliases on the command line

Use `--alias name=tag-name:tag-value` (repeatable). CLI aliases are merged with config-file aliases and take precedence over same-named entries.

```shell
# Single alias
$ssm-session-client shell devbox --alias devbox=Name:my-devbox

# Multiple aliases
$ssm-session-client shell prod-web \
  --alias devbox=Name:my-devbox \
  --alias prod-web=Environment:production

# Combined with a config file (CLI alias overrides config-file alias of the same name)
$ssm-session-client shell devbox --config=config.yaml --alias devbox=Name:other-box
```

#### Using aliases with any command

Once defined, use the alias name as the target for any command:

```shell
$ssm-session-client shell devbox
$ssm-session-client ssh-direct ec2-user@devbox
$ssm-session-client port-forwarding devbox:443 8443
```

## Data Channel Encryption

When KMS encryption is enabled for SSM sessions, the data channel is encrypted using AES-256-GCM. Encryption keys are derived from AWS KMS using the session ID and target ID as encryption context. This requires the AWS Session Manager plugin to be installed.

## Automatic Reconnection

The client supports automatic reconnection when WebSocket connections are interrupted. This can be configured via:

- `--enable-reconnect` (default: `true`): Enable/disable automatic reconnection
- `--max-reconnects` (default: `5`): Maximum number of reconnection attempts (0 = unlimited)

## Building from source

To build this Go project, ensure you have Go installed on your system. You can download and install it from the [official Go website](https://golang.org/dl/).

1. Clone the repository:

```shell
git clone https://github.com/alexbacchin/ssm-session-client.git
cd ssm-session-client
```

2. Build the project for different operating systems:

For Linux:

```shell
GOOS=linux GOARCH=amd64 go build -o ssm-session-client-linux main.go
```

For macOS:

```shell
GOOS=darwin GOARCH=amd64 go build -o ssm-session-client-macos main.go
```

For Windows:

```shell
GOOS=windows GOARCH=amd64 go build -o ssm-session-client.exe main.go
```

This will create an executable file named `ssm-session-client` in the current directory.

You can now use the `ssm-session-client` executable as described in the sections above.
