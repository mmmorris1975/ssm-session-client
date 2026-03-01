package cmd

import (
	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var ssmSshDirectCmd = &cobra.Command{
	Use:   "ssh-direct [user@]target[:port]",
	Short: "SSH directly to an EC2 instance via SSM",
	Long: `Start a direct SSH session to an EC2 instance through AWS SSM Session Manager.

Unlike the 'ssh' command which acts as a proxy for an external SSH client,
ssh-direct provides a fully integrated SSH experience with no external dependencies.

Use --instance-connect to skip key management entirely: an ephemeral Ed25519 key
pair is generated in memory, the public key is pushed to the instance via EC2
Instance Connect (valid for 60 seconds), and the matching private key is used
for authentication automatically.

Examples:
  ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0
  ssm-session-client ssh-direct i-0123456789abcdef0:2222
  ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --exec "uptime"
  ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --ssh-key ~/.ssh/my-key
  ssm-session-client ssh-direct ec2-user@i-0123456789abcdef0 --instance-connect`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		session.InitializeClient()
		if err := session.StartSSHDirectSession(args[0]); err != nil {
			zap.S().Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(ssmSshDirectCmd)

	ssmSshDirectCmd.Flags().StringVar(&config.Flags().SSHKeyFile, "ssh-key", "", "Path to SSH private key file (default: auto-discover)")
	ssmSshDirectCmd.Flags().BoolVar(&config.Flags().NoHostKeyCheck, "no-host-key-check", false, "Skip host key verification (warning: disables MITM protection)")
	ssmSshDirectCmd.Flags().StringVar(&config.Flags().SSHExecCommand, "exec", "", "Execute command instead of starting an interactive shell")
	ssmSshDirectCmd.Flags().BoolVar(&config.Flags().UseInstanceConnect, "instance-connect", false, "Push a temporary SSH key via EC2 Instance Connect (no key files needed)")
	ssmSshDirectCmd.Flags().BoolVar(&config.Flags().NoInstanceConnect, "no-instance-connect", false, "Disable EC2 Instance Connect even if enabled in config file")

	// Bind flags to Viper so preRun's viper.Unmarshal preserves their values.
	viper.BindPFlag("ssh-key-file", ssmSshDirectCmd.Flags().Lookup("ssh-key"))                    //nolint:errcheck
	viper.BindPFlag("no-host-key-check", ssmSshDirectCmd.Flags().Lookup("no-host-key-check"))     //nolint:errcheck
	viper.BindPFlag("ssh-exec-command", ssmSshDirectCmd.Flags().Lookup("exec"))                   //nolint:errcheck
	viper.BindPFlag("instance-connect", ssmSshDirectCmd.Flags().Lookup("instance-connect"))       //nolint:errcheck
	viper.BindPFlag("no-instance-connect", ssmSshDirectCmd.Flags().Lookup("no-instance-connect")) //nolint:errcheck
}
