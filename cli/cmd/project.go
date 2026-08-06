package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var projectDeleteYes bool

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage remote projects",
}

var projectRenameCmd = &cobra.Command{
	Use:     "rename <name> [slug]",
	Aliases: []string{"mv"},
	Short:   "Rename a remote project",
	Long: "Rename a remote project. The slug is immutable and stays the same. " +
		"Renames the current project when slug is omitted.",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		if name == "" {
			return fmt.Errorf("project name is required")
		}
		slug := ""
		if len(args) > 1 {
			slug = args[1]
		} else {
			config, err := loadProjectConfig()
			if err != nil {
				return err
			}
			slug = config.ProjectSlug
		}
		client, err := loadClient()
		if err != nil {
			return err
		}
		project, err := client.UpdateProjectName(slug, name)
		if err != nil {
			return err
		}
		success(cmd.OutOrStdout(), fmt.Sprintf("Renamed project %q to %q.", project.Slug, project.Name))
		return nil
	},
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
		success(cmd.OutOrStdout(), fmt.Sprintf("Deleted remote project %q.", slug))
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
	projectCmd.AddCommand(projectRenameCmd)
	projectCmd.AddCommand(projectDeleteCmd)
}
