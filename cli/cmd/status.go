package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show local project and login status",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		config, configErr := readProjectConfigIfPresent()
		credentials, credentialsErr := readCredentialsIfPresent()

		if configErr != nil {
			fmt.Fprintf(out, "Project: not initialized (%v)\n", configErr)
		} else {
			fmt.Fprintf(out, "Project: %s\n", config.ProjectSlug)
			fmt.Fprintf(out, "Active env: %s\n", resolveEnvironment("", config))
		}

		apiURL := apiBaseURL()
		if os.Getenv("OBE_API_URL") == "" && credentials != nil && credentials.APIURL != "" {
			apiURL = credentials.APIURL
		}
		fmt.Fprintf(out, "API: %s\n", apiURL)

		if credentialsErr != nil {
			fmt.Fprintf(out, "Logged in: no (%v)\n", credentialsErr)
			return nil
		}
		fmt.Fprintln(out, "Logged in: yes")

		if configErr != nil {
			return nil
		}

		client, err := loadClient()
		if err != nil {
			fmt.Fprintf(out, "Remote envs: unavailable (%v)\n", err)
			return nil
		}
		resp, err := client.List(config.ProjectSlug)
		if err != nil {
			fmt.Fprintf(out, "Remote envs: unavailable (%v)\n", err)
			return nil
		}
		if len(resp.Environments) == 0 {
			fmt.Fprintln(out, "Remote envs: none")
			return nil
		}
		fmt.Fprintf(out, "Remote envs: %s\n", strings.Join(resp.Environments, ", "))
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
