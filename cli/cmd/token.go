package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tokenNewName string
var tokenNewDays int

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage API tokens",
}

var tokenListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List API tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := loadClient()
		if err != nil {
			return err
		}
		tokens, err := client.ListTokens()
		if err != nil {
			return err
		}
		rows := make([][]string, 0, len(tokens))
		for _, token := range tokens {
			expiry := "never"
			if token.ExpiresAt != nil {
				expiry = *token.ExpiresAt
			}
			rows = append(rows, []string{token.ID, token.Name, token.CreatedAt, expiry})
		}
		printTable(cmd.OutOrStdout(), []string{"ID", "Name", "Created", "Expires"}, rows)
		return nil
	},
}

var tokenNewCmd = &cobra.Command{
	Use:     "new",
	Aliases: []string{"create"},
	Short:   "Create a new API token",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := promptRequired("Token name", tokenNewName, "")
		if err != nil {
			return err
		}
		var expiresInDays *int
		if tokenNewDays > 0 {
			expiresInDays = &tokenNewDays
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		resp, err := client.CreateToken(name, expiresInDays)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), resp.Token)
		return nil
	},
}

var tokenRmCmd = &cobra.Command{
	Use:     "rm <id>",
	Aliases: []string{"revoke"},
	Short:   "Revoke an API token",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := loadClient()
		if err != nil {
			return err
		}
		if err := client.RevokeToken(args[0]); err != nil {
			return err
		}
		success(cmd.OutOrStdout(), fmt.Sprintf("Revoked token %q.", args[0]))
		return nil
	},
}

func init() {
	tokenNewCmd.Flags().StringVarP(&tokenNewName, "name", "n", "", "Token name")
	tokenNewCmd.Flags().IntVar(&tokenNewDays, "days", 0, "Expire the token after this many days")
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenNewCmd)
	tokenCmd.AddCommand(tokenRmCmd)
}
