package cmd

import (
	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
)

var ec2InstanceConnectCmd = &cobra.Command{
	Use:   "instance-connect [target]",
	Short: "Start a SSH Session using instance connect.",
	Long:  `Start a SSH Session via AWS SSM Session Manager and using instance connect.`,
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		session.InitializeClient()
		session.StartEC2InstanceConnect(args[0])
		return nil
	},
}

func init() {
	ec2InstanceConnectCmd.Flags().StringVar(&config.Flags().SSHPublicKeyFile, "ssh-public-key-file", "", "SSH public key that will be send via EC2 Instance Connect")
	rootCmd.AddCommand(ec2InstanceConnectCmd)
}
