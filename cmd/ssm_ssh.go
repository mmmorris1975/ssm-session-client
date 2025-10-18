package cmd

import (
	"github.com/alexbacchin/ssm-session-client/pkg"
	"github.com/spf13/cobra"
)

var ssmSshCmd = &cobra.Command{
	Use:   "ssh [target]",
	Short: "Start a SSH Session",
	Long:  `Start a SSH Session via AWS SSM Session Manager`,
	Args:  cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		pkg.InitializeClient()
		pkg.StartSSHSession(args[0])
	},
}

func init() {
	rootCmd.AddCommand(ssmSshCmd)
}
