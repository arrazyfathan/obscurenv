package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var swapKey string

var swapCmd = &cobra.Command{
	Use:   "swap <target_env>",
	Short: "Push current .env, switch active environment, and pull target",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := args[0]
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		current := config.ActiveEnvironment
		fmt.Fprintf(cmd.OutOrStdout(), "Switching environment to %q...\n", target)
		if _, err := pushEnvironment(current, swapKey); err != nil {
			return fmt.Errorf("push current environment %q: %w", current, err)
		}
		config.ActiveEnvironment = target
		if err := saveProjectConfig(*config); err != nil {
			return err
		}
		if _, err := pullEnvironment(target, swapKey, true); err != nil {
			config.ActiveEnvironment = current
			_ = saveProjectConfig(*config)
			return fmt.Errorf("pull target environment %q: %w", target, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Local .env now uses %q.\n", target)
		return nil
	},
}

func init() {
	swapCmd.Flags().StringVarP(&swapKey, "key", "k", "", "Encryption passphrase")
}
