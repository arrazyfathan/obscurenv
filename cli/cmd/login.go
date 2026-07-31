package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loginToken string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Store an API token for CLI auth",
	RunE: func(cmd *cobra.Command, args []string) error {
		if loginToken == "" {
			return fmt.Errorf("--token is required")
		}
		if err := saveCredentials(Credentials{Token: loginToken}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Token saved.")
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "API token")
}
