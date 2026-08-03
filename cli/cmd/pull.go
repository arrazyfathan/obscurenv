package cmd

import (
	"fmt"
	"os"

	obvcrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
	"github.com/spf13/cobra"
)

var pullKey string
var pullEnv string

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Download, decrypt, and write .env",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		environment := resolveEnvironment(pullEnv, config)
		passphrase, err := promptSecret("Encryption passphrase", pullKey)
		if err != nil {
			return err
		}
		if _, err := pullEnvironment(environment, passphrase, true); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Pulled %q into .env.\n", environment)
		return nil
	},
}

func init() {
	pullCmd.Flags().StringVarP(&pullKey, "key", "k", "", "Encryption passphrase")
	pullCmd.Flags().StringVarP(&pullEnv, "env", "e", "", "Environment name")
}

func pullEnvironment(environment, passphrase string, writeFile bool) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("encryption passphrase is required")
	}
	config, err := loadProjectConfig()
	if err != nil {
		return nil, err
	}
	client, err := loadClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.Pull(config.ProjectSlug, environment)
	if err != nil {
		return nil, err
	}
	plaintext, err := obvcrypto.DecryptWithPassphrase(resp.EncryptedPayload, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed; .env was not modified: %w", err)
	}
	if !writeFile {
		return plaintext, nil
	}
	tmp := ".env.obe.tmp"
	if err := os.WriteFile(tmp, plaintext, 0600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, ".env"); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	return plaintext, nil
}
