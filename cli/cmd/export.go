package cmd

import (
	"fmt"
	"os"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	obecrypto "github.com/obscurenv/obscurenv/cli/pkg/crypto"
	"github.com/spf13/cobra"
)

var exportKey string
var exportSlug string

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Download and decrypt all environments to local env files",
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := exportSlug
		if slug == "" {
			config, err := loadProjectConfig()
			if err != nil {
				return err
			}
			slug = config.ProjectSlug
		}
		passphrase, err := promptSecret("Encryption passphrase", exportKey)
		if err != nil {
			return err
		}
		if passphrase == "" {
			return fmt.Errorf("encryption passphrase is required")
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		var resp *api.ExportResponse
		if err := withSpinner(cmd.OutOrStdout(), "Exporting "+slug, func() error {
			var eerr error
			resp, eerr = client.Export(slug)
			return eerr
		}); err != nil {
			return err
		}
		if len(resp.Environments) == 0 {
			return fmt.Errorf("project %q has no environments", slug)
		}

		decoded := make([]struct {
			environment string
			file        string
			plaintext   []byte
		}, 0, len(resp.Environments))
		for _, item := range resp.Environments {
			file, err := validateManagedFile(item.Environment + ".env")
			if err != nil {
				return fmt.Errorf("invalid environment name %q: %w", item.Environment, err)
			}
			plaintext, err := obecrypto.DecryptWithPassphrase(item.EncryptedPayload, passphrase)
			if err != nil {
				return fmt.Errorf("decrypt %q failed; no files were modified: %w", item.Environment, err)
			}
			decoded = append(decoded, struct {
				environment string
				file        string
				plaintext   []byte
			}{environment: item.Environment, file: file, plaintext: plaintext})
		}

		for _, env := range decoded {
			tmp := env.file + ".obe.tmp"
			if err := os.WriteFile(tmp, env.plaintext, 0600); err != nil {
				return err
			}
			if err := os.Rename(tmp, env.file); err != nil {
				_ = os.Remove(tmp)
				return err
			}
			success(cmd.OutOrStdout(), fmt.Sprintf("Exported %q into %s.", env.environment, env.file))
		}
		return nil
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportKey, "key", "k", "", "Encryption passphrase")
	exportCmd.Flags().StringVarP(&exportSlug, "project", "p", "", "Project slug")
}
