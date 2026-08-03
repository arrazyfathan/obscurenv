package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	"github.com/spf13/cobra"
)

var initProject string
var initProjectName string
var initCreateProject bool
var initEnvironment string
var initFile string

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
			resp, created, err := createOrLinkProject(client, api.CreateProjectRequest{Name: name, Slug: project})
			if err != nil {
				return fmt.Errorf("create project: %w", err)
			}
			project = resp.Slug
			if created {
				fmt.Fprintf(cmd.OutOrStdout(), "Created remote project %q.\n", project)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Remote project %q already exists; linked local config.\n", project)
			}
		}
		envFile := ""
		if initFile != "" {
			envFile, err = validateManagedFile(initFile)
			if err != nil {
				return err
			}
		}
		config := ProjectConfig{
			ProjectSlug:       initProject,
			ActiveEnvironment: environment,
			EnvFile:           envFile,
		}
		config.ProjectSlug = project
		if err := saveProjectConfig(config); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s for project %q using %q.\n", projectConfigFile, project, environment)
		return nil
	},
}

func createOrLinkProject(client *api.Client, req api.CreateProjectRequest) (*api.CreateProjectResponse, bool, error) {
	resp, err := client.CreateProject(req)
	if err == nil {
		return resp, true, nil
	}
	if !api.IsStatus(err, http.StatusConflict) {
		return nil, false, err
	}
	project, getErr := client.GetProject(req.Slug)
	if getErr != nil {
		return nil, false, fmt.Errorf("%w; failed to verify existing project: %v", err, getErr)
	}
	return &api.CreateProjectResponse{
		ID:   project.ID,
		Slug: project.Slug,
	}, false, nil
}

func init() {
	initCmd.Flags().StringVarP(&initProject, "project", "p", "", "Project slug")
	initCmd.Flags().StringVar(&initProjectName, "name", "", "Project display name when creating it remotely")
	initCmd.Flags().StringVarP(&initEnvironment, "env", "e", "", "Initial active environment")
	initCmd.Flags().StringVar(&initFile, "file", "", "Managed local file path, such as .env or local.properties")
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
