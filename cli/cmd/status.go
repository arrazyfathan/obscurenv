package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show local project and login status",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		config, configErr := readProjectConfigIfPresent()
		credentials, credentialsErr := readCredentialsIfPresent()

		rows := make([][]string, 0, 6)
		if configErr != nil {
			rows = append(rows, []string{"Project", "not initialized (" + configErr.Error() + ")"})
		} else {
			rows = append(rows, []string{"Project", config.ProjectSlug})
			rows = append(rows, []string{"Active env", resolveEnvironment("", config)})
			if config.EnvFile != "" {
				rows = append(rows, []string{"Managed file", config.EnvFile})
			}
		}

		apiURL := apiBaseURL()
		if os.Getenv("OBE_API_URL") == "" && credentials != nil && credentials.APIURL != "" {
			apiURL = credentials.APIURL
		}
		rows = append(rows, []string{"API", apiURL})

		if credentialsErr != nil {
			rows = append(rows, []string{"Logged in", "no (" + credentialsErr.Error() + ")"})
			printTable(out, []string{"Key", "Value"}, rows)
			return nil
		}
		rows = append(rows, []string{"Logged in", "yes"})

		if configErr != nil {
			printTable(out, []string{"Key", "Value"}, rows)
			return nil
		}

		client, err := loadClient()
		if err != nil {
			rows = append(rows, []string{"Remote envs", "unavailable (" + err.Error() + ")"})
			printTable(out, []string{"Key", "Value"}, rows)
			return nil
		}
		project, err := validateLinkedProject(config, client)
		if err != nil {
			if config.ProjectID == "" {
				rows = append(rows, []string{"Remote project", err.Error()})
			} else if api.IsStatus(err, http.StatusNotFound) || strings.Contains(err.Error(), "no longer owned") {
				rows = append(rows, []string{"Remote project", "access lost; run obe init to relink"})
			} else {
				rows = append(rows, []string{"Remote project", err.Error()})
			}
			printTable(out, []string{"Key", "Value"}, rows)
			if strings.Contains(err.Error(), "access lost") || strings.Contains(err.Error(), "stale") {
				return err
			}
			return nil
		}
		if config.ProjectID == "" {
			// Legacy configs are not identity-bound, so verify access explicitly
			// here to detect removal and to report the owner/collaborator role.
			linked, getErr := client.GetProject(config.ProjectSlug)
			if getErr != nil {
				if api.IsStatus(getErr, http.StatusNotFound) {
					rows = append(rows, []string{"Remote project", "access lost; run obe init to relink"})
					printTable(out, []string{"Key", "Value"}, rows)
					return fmt.Errorf("project %q is no longer accessible; run obe init to relink this directory", config.ProjectSlug)
				}
				rows = append(rows, []string{"Remote project", "unavailable (" + getErr.Error() + ")"})
				printTable(out, []string{"Key", "Value"}, rows)
				return nil
			}
			project = linked
			rows = append(rows, []string{"Remote project", "linked"})
			rows = append(rows, []string{"Role", projectRoleLabel(project.AccessLevel)})
			rows = append(rows, []string{"Note", "legacy config; run obe init --relink --project " + config.ProjectSlug + " to upgrade"})
		} else if project == nil {
			rows = append(rows, []string{"Remote project", "unavailable"})
		} else {
			rows = append(rows, []string{"Role", projectRoleLabel(project.AccessLevel)})
		}
		resp, err := client.List(config.ProjectSlug)
		if err != nil {
			rows = append(rows, []string{"Remote envs", "unavailable (" + err.Error() + ")"})
			printTable(out, []string{"Key", "Value"}, rows)
			return nil
		}
		if len(resp.Environments) == 0 {
			rows = append(rows, []string{"Remote envs", "none"})
			printTable(out, []string{"Key", "Value"}, rows)
			return nil
		}
		rows = append(rows, []string{"Remote envs", strings.Join(resp.Environments, ", ")})
		printTable(out, []string{"Key", "Value"}, rows)
		return nil
	},
}

func readProjectConfigIfPresent() (*ProjectConfig, error) {
	data, err := os.ReadFile(projectConfigFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("run: obe init")
		}
		return nil, fmt.Errorf("read %s: %w", projectConfigFile, err)
	}
	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", projectConfigFile, err)
	}
	if config.ProjectSlug == "" {
		return nil, fmt.Errorf("%s is missing project_slug", projectConfigFile)
	}
	if config.ActiveEnvironment == "" {
		config.ActiveEnvironment = "development"
	}
	return &config, nil
}

func readCredentialsIfPresent() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("run: obe login")
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var credentials Credentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if credentials.Token == "" {
		return nil, fmt.Errorf("credentials missing token")
	}
	return &credentials, nil
}

func projectRoleLabel(accessLevel string) string {
	if accessLevel == "owner" {
		return "owner"
	}
	return "collaborator"
}
