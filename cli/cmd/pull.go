package cmd

import (
	"fmt"
	"os"

	obecrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
	"github.com/spf13/cobra"
)

var pullKey string
var pullEnv string
var pullFile string

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Download, decrypt, and write a local env file",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		if _, err := requireLinkedProject(config); err != nil {
			return err
		}
		environment := resolveEnvironment(pullEnv, config)
		file, err := resolveManagedFile(pullFile, config, false)
		if err != nil {
			return err
		}
		passphrase, err := promptSecret("Encryption passphrase", pullKey)
		if err != nil {
			return err
		}
		if err := withSpinner(cmd.OutOrStdout(), "Pulling "+environment, func() error {
			_, perr := pullEnvironment(environment, passphrase, file, true)
			return perr
		}); err != nil {
			return err
		}
		success(cmd.OutOrStdout(), fmt.Sprintf("Pulled %q into %s.", environment, file))
		return nil
	},
}

func init() {
	pullCmd.Flags().StringVarP(&pullKey, "key", "k", "", "Encryption passphrase")
	pullCmd.Flags().StringVarP(&pullEnv, "env", "e", "", "Environment name")
	pullCmd.Flags().StringVar(&pullFile, "file", "", "Managed local file path, such as .env or local.properties")
}

func pullEnvironment(environment, passphrase, file string, writeFile bool) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("encryption passphrase is required")
	}
	file, err := validateManagedFile(file)
	if err != nil {
		return nil, err
	}
	config, err := loadProjectConfig()
	if err != nil {
		return nil, err
	}
	if _, err := requireLinkedProject(config); err != nil {
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
	plaintext, err := obecrypto.DecryptWithPassphrase(resp.EncryptedPayload, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed; %s was not modified: %w", file, err)
	}
	if !writeFile {
		return plaintext, nil
	}
	if isLocalPropertiesFile(file) {
		existing, err := os.ReadFile(file)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			plaintext = mergeLocalOnlyProperties(plaintext, existing)
		}
	}
	tmp := file + ".obe.tmp"
	if err := os.WriteFile(tmp, plaintext, 0600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := rememberManagedFile(config, file); err != nil {
		return nil, err
	}
	return plaintext, nil
}
