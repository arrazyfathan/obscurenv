package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	obvcrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
	"github.com/spf13/cobra"
)

var pushKey string
var pushEnv string

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Encrypt local .env and upload it",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		environment := resolveEnvironment(pushEnv, config)
		version, err := pushEnvironment(environment, pushKey)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Pushed %q as version %d.\n", environment, version)
		return nil
	},
}

func init() {
	pushCmd.Flags().StringVarP(&pushKey, "key", "k", "", "Encryption passphrase")
	pushCmd.Flags().StringVarP(&pushEnv, "env", "e", "", "Environment name")
}

func pushEnvironment(environment, passphrase string) (int, error) {
	if passphrase == "" {
		return 0, fmt.Errorf("--key is required")
	}
	config, err := loadProjectConfig()
	if err != nil {
		return 0, err
	}
	plaintext, err := os.ReadFile(".env")
	if err != nil {
		return 0, fmt.Errorf("read .env: %w", err)
	}
	payload, err := obvcrypto.EncryptWithPassphrase(plaintext, passphrase)
	if err != nil {
		return 0, err
	}
	sum := sha256.Sum256(plaintext)
	client, err := loadClient()
	if err != nil {
		return 0, err
	}
	resp, err := client.Push(api.PushRequest{
		ProjectSlug:      config.ProjectSlug,
		Environment:      environment,
		EncryptedPayload: payload,
		Checksum:         hex.EncodeToString(sum[:]),
	})
	if err != nil {
		return 0, err
	}
	return resp.Version, nil
}
