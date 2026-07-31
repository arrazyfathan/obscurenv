package cmd

import (
	"fmt"
	"strings"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	"github.com/spf13/cobra"
)

var initProject string
var initProjectName string
var initCreateProject bool
var initEnvironment string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Connect the current directory to an Obscurenv project",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := promptRequired("Project slug", initProject, "")
		if err != nil {
			return err
		}
		environment, err := promptRequired("Active environment", initEnvironment, "development")
		if err != nil {
			return err
		}
		createProject := initCreateProject
		if !createProject && stdinIsTerminal() {
			createProject, err = promptConfirm("Create this project on the server?", true)
			if err != nil {
				return err
			}
		}
		if createProject {
			nameFallback := titleFromSlug(project)
			name, err := promptRequired("Project name", initProjectName, nameFallback)
			if err != nil {
				return err
			}
			client, err := loadClient()
			if err != nil {
				return err
			}
			resp, err := client.CreateProject(api.CreateProjectRequest{
				Name: name,
				Slug: project,
			})
			if err != nil {
				return fmt.Errorf("create project: %w", err)
			}
			project = resp.Slug
			fmt.Fprintf(cmd.OutOrStdout(), "Created remote project %q.\n", project)
		}
		config := ProjectConfig{
			ProjectSlug:       initProject,
			ActiveEnvironment: environment,
		}
		config.ProjectSlug = project
		if err := saveProjectConfig(config); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s for project %q using %q.\n", projectConfigFile, project, environment)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initProject, "project", "p", "", "Project slug")
	initCmd.Flags().StringVar(&initProjectName, "name", "", "Project display name when creating it remotely")
	initCmd.Flags().StringVarP(&initEnvironment, "env", "e", "", "Initial active environment")
	initCmd.Flags().BoolVar(&initCreateProject, "create", false, "Create the project on the server")
}

func titleFromSlug(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
