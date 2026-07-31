package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

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
		fmt.Fprintf(cmd.OutOrStdout(), "Active environment set to %q.\n", config.ActiveEnvironment)
		return nil
	},
}

func init() {
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envUseCmd)
}
