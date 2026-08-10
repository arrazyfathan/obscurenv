package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	obecrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
	"github.com/spf13/cobra"
)

var pushKey string
var pushEnv string
var pushFile string

var pushCmd = &cobra.Command{
	Use:   "push [file]",
	Short: "Encrypt a local env file and upload it",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		if _, err := requireLinkedProject(config); err != nil {
			return err
		}
		environment := resolveEnvironment(pushEnv, config)
		file, err := resolvePushManagedFile(pushFile, args, config)
		if err != nil {
			return err
		}
		passphrase, err := promptSecret("Encryption passphrase", pushKey)
		if err != nil {
			return err
		}
		var version int
		if err := withSpinner(cmd.OutOrStdout(), "Pushing "+environment, func() error {
			var perr error
			version, perr = pushEnvironment(environment, passphrase, file)
			return perr
		}); err != nil {
			return err
		}
		success(cmd.OutOrStdout(), fmt.Sprintf("Pushed %q from %s as version %d.", environment, file, version))
		return nil
	},
}

func init() {
	pushCmd.Flags().StringVarP(&pushKey, "key", "k", "", "Encryption passphrase")
	pushCmd.Flags().StringVarP(&pushEnv, "env", "e", "", "Environment name")
	pushCmd.Flags().StringVar(&pushFile, "file", "", "Managed local file path, such as .env or local.properties")
}

func resolvePushManagedFile(flagValue string, args []string, config *ProjectConfig) (string, error) {
	if flagValue != "" {
		return validateManagedFile(flagValue)
	}
	if len(args) > 0 {
		return validateManagedFile(args[0])
	}

	file, err := autoDetectManagedFile(true)
	if err == nil {
		return file, nil
	}
	if config != nil && config.EnvFile != "" {
		return validateManagedFile(config.EnvFile)
	}
	return "", err
}

func pushEnvironment(environment, passphrase, file string) (int, error) {
	if passphrase == "" {
		return 0, fmt.Errorf("encryption passphrase is required")
	}
	file, err := validateManagedFile(file)
	if err != nil {
		return 0, err
	}
	config, err := loadProjectConfig()
	if err != nil {
		return 0, err
	}
	if _, err := requireLinkedProject(config); err != nil {
		return 0, err
	}
	plaintext, err := os.ReadFile(file)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", file, err)
	}
	if isLocalPropertiesFile(file) {
		plaintext = stripLocalOnlyProperties(plaintext)
	}
	payload, err := obecrypto.EncryptWithPassphrase(plaintext, passphrase)
	if err != nil {
		return 0, err
	}
	sum := sha256.Sum256([]byte(payload))
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
	if err := rememberManagedFile(config, file); err != nil {
		return 0, err
	}
	return resp.Version, nil
}
