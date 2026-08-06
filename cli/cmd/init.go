package cmd

import (
	"fmt"
	"net/http"
	"os"
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
		if config, err := readProjectConfigIfPresent(); err == nil {
			return reportExistingInit(cmd, config)
		} else if !isMissingProjectConfig(err) {
			return err
		}

		project, err := promptRequired("Project slug", initProject, "")
		if err != nil {
			return err
		}

		client, err := loadClient()
		if err != nil {
			return err
		}

		var linked *api.ProjectResponse
		if err := withSpinner(cmd.OutOrStdout(), "Checking project "+project, func() error {
			var lerr error
			linked, lerr = linkExistingProject(client, project)
			return lerr
		}); err != nil {
			if !api.IsStatus(err, http.StatusNotFound) {
				return fmt.Errorf("check project: %w", err)
			}
		}
		if linked != nil {
			environment, err := chooseLinkedProjectEnvironment(cmd, client, linked.Slug)
			if err != nil {
				return err
			}
			if err := writeInitConfig(linked.Slug, environment); err != nil {
				return err
			}
			success(cmd.OutOrStdout(), fmt.Sprintf("Linked local config to remote project %q.", linked.Slug))
			success(cmd.OutOrStdout(), fmt.Sprintf("Created %s for project %q using %q.", projectConfigFile, linked.Slug, environment))
			return nil
		}

		nameFallback := titleFromSlug(project)
		name, err := promptRequired("Project name", initProjectName, nameFallback)
		if err != nil {
			return err
		}
		environment, err := promptRequired("Active environment", initEnvironment, "development")
		if err != nil {
			return err
		}

		var resp *api.CreateProjectResponse
		var created bool
		if err := withSpinner(cmd.OutOrStdout(), "Setting up project "+project, func() error {
			var cerr error
			resp, created, cerr = createOrLinkProject(client, api.CreateProjectRequest{Name: name, Slug: project})
			return cerr
		}); err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		project = resp.Slug
		if created {
			success(cmd.OutOrStdout(), fmt.Sprintf("Created remote project %q.", project))
		} else {
			info(cmd.OutOrStdout(), fmt.Sprintf("Remote project %q already exists; linked local config.", project))
		}
		if err := writeInitConfig(project, environment); err != nil {
			return err
		}
		success(cmd.OutOrStdout(), fmt.Sprintf("Created %s for project %q using %q.", projectConfigFile, project, environment))
		return nil
	},
}

func reportExistingInit(cmd *cobra.Command, config *ProjectConfig) error {
	out := cmd.OutOrStdout()
	info(out, "Already initialized.")
	fmt.Fprintf(out, "Project: %s\n", config.ProjectSlug)
	fmt.Fprintf(out, "Active env: %s\n", resolveEnvironment("", config))
	if config.EnvFile != "" {
		fmt.Fprintf(out, "Managed file: %s\n", config.EnvFile)
	}

	client, err := loadClient()
	if err != nil {
		fmt.Fprintf(out, "Remote: unavailable (%v)\n", err)
		return nil
	}
	if _, err := linkExistingProject(client, config.ProjectSlug); err != nil {
		if api.IsStatus(err, http.StatusNotFound) {
			fmt.Fprintln(out, "Remote: not linked")
			return nil
		}
		fmt.Fprintf(out, "Remote: unavailable (%v)\n", err)
		return nil
	}
	fmt.Fprintln(out, "Remote: linked")
	return nil
}

func linkExistingProject(client *api.Client, slug string) (*api.ProjectResponse, error) {
	project, err := client.GetProject(slug)
	if err != nil {
		return nil, err
	}
	return project, nil
}

func chooseLinkedProjectEnvironment(cmd *cobra.Command, client *api.Client, projectSlug string) (string, error) {
	if initEnvironment != "" {
		return initEnvironment, nil
	}
	resp, err := client.List(projectSlug)
	if err != nil {
		return "", fmt.Errorf("list remote environments: %w", err)
	}
	if len(resp.Environments) == 0 {
		return "development", nil
	}
	return chooseEnvironmentFromList(cmd, resp.Environments, "")
}

func chooseEnvironmentFromList(cmd *cobra.Command, environments []string, current string) (string, error) {
	if interactive() {
		return interactiveSelect("Select an environment", environments, current)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Available environments:")
	for i, environment := range environments {
		marker := ""
		if environment == current {
			marker = " (current)"
		}
		fmt.Fprintf(out, "  %d. %s%s\n", i+1, environment, marker)
	}
	if !stdinIsTerminal() {
		return "", fmt.Errorf("environment is required; pass one of the listed environments with --env")
	}
	value, err := promptString("Use environment", current)
	if err != nil {
		return "", err
	}
	return resolveEnvironmentChoice(value, environments)
}

func writeInitConfig(project, environment string) error {
	envFile, err := resolveManagedFile(initFile, nil, false)
	if err != nil {
		return err
	}
	config := ProjectConfig{
		ProjectSlug:       project,
		ActiveEnvironment: environment,
		EnvFile:           envFile,
	}
	if err := saveProjectConfig(config); err != nil {
		return err
	}
	return ensureProjectConfigIgnored()
}

func ensureProjectConfigIgnored() error {
	const gitignoreFile = ".gitignore"

	data, err := os.ReadFile(gitignoreFile)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(gitignoreFile, []byte(projectConfigFile+"\n"), 0600)
		}
		return fmt.Errorf("read %s: %w", gitignoreFile, err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == projectConfigFile {
			return nil
		}
	}

	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += projectConfigFile + "\n"
	return os.WriteFile(gitignoreFile, []byte(content), 0600)
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

func isMissingProjectConfig(err error) bool {
	return err != nil && strings.Contains(err.Error(), "run: obe init")
}

func init() {
	initCmd.Flags().StringVarP(&initProject, "project", "p", "", "Project slug")
	initCmd.Flags().StringVar(&initProjectName, "name", "", "Project display name when creating it remotely")
	initCmd.Flags().StringVarP(&initEnvironment, "env", "e", "", "Initial active environment")
	initCmd.Flags().StringVar(&initFile, "file", "", "Managed local file path, such as .env or local.properties")
	initCmd.Flags().BoolVar(&initCreateProject, "create", false, "Accepted for compatibility; missing projects are created during init")
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
