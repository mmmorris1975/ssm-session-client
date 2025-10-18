package cmd

import (
	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/pkg"
	"github.com/spf13/cobra"
)

var ec2InstanceConnectCmd = &cobra.Command{
	Use:   "instance-connect [target]",
	Short: "Start a SSH Session using instance connect.",
	Long:  `Start a SSH Session via AWS SSM Session Manager and using instance connect.`,
	Args:  cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		pkg.InitializeClient()
		pkg.StartEC2InstanceConnect(args[0])
	},
}

func init() {
	ec2InstanceConnectCmd.Flags().StringVar(&config.Flags().SSHPublicKeyFile, "ssh-public-key-file", "", "SSH public key that will be send via EC2 Instance Connect")
	rootCmd.AddCommand(ec2InstanceConnectCmd)
}
