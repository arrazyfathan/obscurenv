package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initProject string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Connect the current directory to an Obscurenv project",
	RunE: func(cmd *cobra.Command, args []string) error {
		if initProject == "" {
			return fmt.Errorf("--project is required")
		}
		config := ProjectConfig{
			ProjectSlug:       initProject,
			ActiveEnvironment: "development",
		}
		if err := saveProjectConfig(config); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s for project %q.\n", projectConfigFile, initProject)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initProject, "project", "p", "", "Project slug")
}
