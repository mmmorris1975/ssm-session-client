package cmd

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var ec2InstanceConnectAddProxyCommandCmd = &cobra.Command{
	Use:   "add-proxy-command [user@]target",
	Short: "Add SSH config entry with instance-connect proxy command",
	Long: `Add an entry to ~/.ssh/config that configures SSH to connect to the target
instance through ssm-session-client instance-connect as a proxy command.

Examples:
  ssm-session-client instance-connect add-proxy-command ec2-user@i-0123456789abcdef0
  ssm-session-client instance-connect add-proxy-command ubuntu@i-0123456789abcdef0 --config /path/to/config.yaml`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := upsertSSHConfigEntry(args[0], "instance-connect"); err != nil {
			zap.S().Fatal(err)
		}
	},
}

func init() {
	ec2InstanceConnectCmd.AddCommand(ec2InstanceConnectAddProxyCommandCmd)
}
