//go:build windows

package cmd

import (
	"fmt"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/alexbacchin/ssm-session-client/session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ssmRDPCmd = &cobra.Command{
	Use:   "rdp [target]",
	Short: "RDP to a Windows EC2 instance via SSM",
	Long: `Start an RDP session to a Windows EC2 instance through AWS SSM Session Manager.

An SSM port-forwarding tunnel is created to the instance's RDP port, then the
native Windows Remote Desktop client (mstsc.exe) is launched to connect through it.

By default mstsc.exe will prompt for credentials. Use --get-password to
automatically retrieve the EC2-generated administrator password using the
instance's key pair.

Examples:
  ssm-session-client rdp i-0123456789abcdef0
  ssm-session-client rdp i-0123456789abcdef0 --username Administrator
  ssm-session-client rdp i-0123456789abcdef0 --get-password --key-pair-file ~/.ssh/my-key.pem
  ssm-session-client rdp i-0123456789abcdef0 --rdp-port 3389 --local-port 13389`,
	Args: cobra.ExactArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if config.Flags().RDPGetPassword && config.Flags().RDPKeyPairFile == "" {
			return fmt.Errorf("--key-pair-file is required when --get-password is set")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		session.InitializeClient()
		return session.StartRDPSession(args[0])
	},
}

func init() {
	rootCmd.AddCommand(ssmRDPCmd)

	ssmRDPCmd.Flags().IntVar(&config.Flags().RDPPort, "rdp-port", 3389, "Remote RDP port on the EC2 instance")
	ssmRDPCmd.Flags().IntVar(&config.Flags().RDPLocalPort, "local-port", 0, "Local port for the SSM tunnel (0 = auto-assign)")
	ssmRDPCmd.Flags().BoolVar(&config.Flags().RDPGetPassword, "get-password", false, "Retrieve EC2 administrator password via AWS API")
	ssmRDPCmd.Flags().StringVar(&config.Flags().RDPKeyPairFile, "key-pair-file", "", "Path to the EC2 key pair private key (required with --get-password)")
	ssmRDPCmd.Flags().StringVar(&config.Flags().RDPUsername, "username", "Administrator", "RDP username")

	ssmRDPCmd.MarkFlagsRequiredTogether("get-password", "key-pair-file") //nolint:errcheck

	viper.BindPFlag("rdp-port", ssmRDPCmd.Flags().Lookup("rdp-port"))             //nolint:errcheck
	viper.BindPFlag("rdp-local-port", ssmRDPCmd.Flags().Lookup("local-port"))     //nolint:errcheck
	viper.BindPFlag("rdp-get-password", ssmRDPCmd.Flags().Lookup("get-password")) //nolint:errcheck
	viper.BindPFlag("rdp-key-pair-file", ssmRDPCmd.Flags().Lookup("key-pair-file")) //nolint:errcheck
	viper.BindPFlag("rdp-username", ssmRDPCmd.Flags().Lookup("username"))         //nolint:errcheck
}
