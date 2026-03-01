package cmd

import (
	"fmt"
	"strconv"

	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
)

var portForwardingCmd = &cobra.Command{
	Use:   "port-forwarding [target:destination port] [source port]",
	Short: "Start a Port Forwarding Shell Session",
	Long:  `Start a Port Forwarding via AWS SSM Session Manager`,
	Args:  cobra.MatchAll(cobra.MinimumNArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		sourcePort, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Println("Invalid source port:", args[1])
			return
		}
		session.InitializeClient()
		session.StartSSMPortForwarder(args[0], sourcePort)
	},
}

func init() {
	rootCmd.AddCommand(portForwardingCmd)
}
