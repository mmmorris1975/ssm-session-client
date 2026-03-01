package cmd

import (
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
)

var ssmSshCmd = &cobra.Command{
	Use:   "ssh [target]",
	Short: "Start a SSH Session",
	Long:  `Start a SSH Session via AWS SSM Session Manager`,
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		session.InitializeClient()
		return session.StartSSHSession(args[0])
	},
}

func init() {
	rootCmd.AddCommand(ssmSshCmd)
}
