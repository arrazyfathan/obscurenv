package cmd

import (
	"fmt"
	"strings"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	"github.com/spf13/cobra"
)

var loginToken string
var loginEmail string
var loginUsername string
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
			email := loginEmail
			username := loginUsername
			if username == "" && email == "" {
				label := "Email or username"
				if loginRegister {
					label = "Email"
				}
				identifier, err := promptRequired(label, "", "")
				if err != nil {
					return err
				}
				email, username = splitLoginIdentifier(identifier)
			}
			if loginRegister && email == "" {
				return fmt.Errorf("email is required to register")
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
					Username: username,
					Password: password,
				}); err != nil {
					return fmt.Errorf("register user: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Registered user.")
			}
			resp, err := client.Login(api.LoginRequest{
				Email:     email,
				Username:  username,
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
	loginCmd.Flags().StringVar(&loginUsername, "username", "", "Account username")
	loginCmd.Flags().StringVar(&loginPassword, "password", "", "Account password")
	loginCmd.Flags().StringVar(&loginTokenName, "token-name", "local-cli", "API token name")
	loginCmd.Flags().StringVar(&loginAPIURL, "api-url", "", "Obscurenv API URL")
	loginCmd.Flags().BoolVar(&loginRegister, "register", false, "Register the user before logging in")
}

func splitLoginIdentifier(identifier string) (email, username string) {
	identifier = strings.TrimSpace(identifier)
	if strings.Contains(identifier, "@") {
		return identifier, ""
	}
	return "", identifier
}
