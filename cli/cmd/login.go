package cmd

import (
	"fmt"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	"github.com/spf13/cobra"
)

var loginToken string
var loginEmail string
var loginPassword string
var loginTokenName string
var loginAPIURL string
var loginRegister bool

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in and store CLI credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiURL, err := promptRequired("API URL", loginAPIURL, apiBaseURL())
		if err != nil {
			return err
		}

		token := loginToken
		if token == "" {
			email, err := promptRequired("Email", loginEmail, "")
			if err != nil {
				return err
			}
			password, err := promptSecret("Password", loginPassword)
			if err != nil {
				return err
			}
			tokenName, err := promptRequired("Token name", loginTokenName, "local-cli")
			if err != nil {
				return err
			}
			client := api.New(apiURL, "")
			if loginRegister {
				if err := client.Register(api.RegisterRequest{
					Email:    email,
					Password: password,
				}); err != nil {
					return fmt.Errorf("register user: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Registered user.")
			}
			resp, err := client.Login(api.LoginRequest{
				Email:     email,
				Password:  password,
				TokenName: tokenName,
			})
			if err != nil {
				return fmt.Errorf("login: %w", err)
			}
			token = resp.Token
		}
		if err := saveCredentials(Credentials{Token: token, APIURL: apiURL}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Logged in. Credentials saved.")
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "API token, if you already have one")
	loginCmd.Flags().StringVar(&loginEmail, "email", "", "Account email")
	loginCmd.Flags().StringVar(&loginPassword, "password", "", "Account password")
	loginCmd.Flags().StringVar(&loginTokenName, "token-name", "local-cli", "API token name")
	loginCmd.Flags().StringVar(&loginAPIURL, "api-url", "", "Obscurenv API URL")
	loginCmd.Flags().BoolVar(&loginRegister, "register", false, "Register the user before logging in")
}
