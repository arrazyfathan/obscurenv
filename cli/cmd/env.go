package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var envDeleteYes bool

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage remote environments",
}

var envListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List remote environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		resp, err := client.List(config.ProjectSlug)
		if err != nil {
			return err
		}
		for _, environment := range resp.Environments {
			fmt.Fprintln(cmd.OutOrStdout(), environment)
		}
		return nil
	},
}

var envUseCmd = &cobra.Command{
	Use:   "use <environment>",
	Short: "Set the active environment without modifying .env",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		config.ActiveEnvironment = args[0]
		if err := saveProjectConfig(*config); err != nil {
			return err
		}
		success(cmd.OutOrStdout(), fmt.Sprintf("Active environment set to %q.", config.ActiveEnvironment))
		return nil
	},
}

var envDeleteCmd = &cobra.Command{
	Use:     "rm <environment>",
	Aliases: []string{"delete", "remove"},
	Short:   "Delete a remote environment from the server",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		environment := args[0]
		if err := confirmRemoteDeletion(envDeleteYes, fmt.Sprintf("Delete remote environment %q from project %q?", environment, config.ProjectSlug)); err != nil {
			return err
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		if err := client.DeleteEnvironment(config.ProjectSlug, environment); err != nil {
			return err
		}
		success(cmd.OutOrStdout(), fmt.Sprintf("Deleted remote environment %q from project %q.", environment, config.ProjectSlug))
		return nil
	},
}

func confirmRemoteDeletion(yes bool, label string) error {
	if yes {
		return nil
	}
	confirmed, err := promptConfirm(label, false)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("delete canceled; pass --yes to confirm")
	}
	return nil
}

func init() {
	envDeleteCmd.Flags().BoolVarP(&envDeleteYes, "yes", "y", false, "Confirm deletion without prompting")
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envUseCmd)
	envCmd.AddCommand(envDeleteCmd)
}
