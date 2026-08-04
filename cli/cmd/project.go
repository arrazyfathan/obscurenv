package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var projectDeleteYes bool

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage remote projects",
}

var projectDeleteCmd = &cobra.Command{
	Use:     "rm [slug]",
	Aliases: []string{"delete", "remove"},
	Short:   "Delete a remote project from the server",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug, err := projectSlugForDelete(args)
		if err != nil {
			return err
		}
		if err := confirmRemoteDeletion(projectDeleteYes, fmt.Sprintf("Delete remote project %q and all of its environments?", slug)); err != nil {
			return err
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		if err := client.DeleteProject(slug); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted remote project %q.\n", slug)
		return nil
	},
}

func projectSlugForDelete(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	config, err := loadProjectConfig()
	if err != nil {
		return "", err
	}
	return config.ProjectSlug, nil
}

func init() {
	projectDeleteCmd.Flags().BoolVarP(&projectDeleteYes, "yes", "y", false, "Confirm deletion without prompting")
	projectCmd.AddCommand(projectDeleteCmd)
}
