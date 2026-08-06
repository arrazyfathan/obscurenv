package cmd

import (
	"github.com/spf13/cobra"
)

var accountYes bool

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage the account",
}

var accountRmCmd = &cobra.Command{
	Use:     "rm",
	Aliases: []string{"delete"},
	Short:   "Delete the account and all remote data",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := confirmRemoteDeletion(accountYes, "Delete your account and all projects and environments?"); err != nil {
			return err
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		if err := client.DeleteAccount(); err != nil {
			return err
		}
		success(cmd.OutOrStdout(), "Account deleted.")
		return nil
	},
}

func init() {
	accountRmCmd.Flags().BoolVarP(&accountYes, "yes", "y", false, "Confirm deletion without prompting")
	accountCmd.AddCommand(accountRmCmd)
}
