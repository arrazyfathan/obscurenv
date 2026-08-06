package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var passwdCurrent string
var passwdNew string

var passwdCmd = &cobra.Command{
	Use:   "passwd",
	Short: "Change the account password",
	RunE: func(cmd *cobra.Command, args []string) error {
		current, err := promptSecret("Current password", passwdCurrent)
		if err != nil {
			return err
		}
		newPassword, err := promptSecret("New password", passwdNew)
		if err != nil {
			return err
		}
		if len(newPassword) < 8 {
			return fmt.Errorf("new password must be at least 8 characters")
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		if err := client.ChangePassword(current, newPassword); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Password updated.")
		return nil
	},
}

func init() {
	passwdCmd.Flags().StringVar(&passwdCurrent, "current", "", "Current password")
	passwdCmd.Flags().StringVar(&passwdNew, "new", "", "New password")
}
