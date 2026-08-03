package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	"github.com/spf13/cobra"
)

const (
	projectConfigFile = ".obe.json"
	defaultAPIURL     = "https://localhost:8080"
)

type ProjectConfig struct {
	ProjectSlug       string `json:"project_slug"`
	ActiveEnvironment string `json:"active_environment"`
}

type Credentials struct {
	Token  string `json:"token"`
	APIURL string `json:"api_url,omitempty"`
}

var rootCmd = &cobra.Command{
	Use:     "obe",
	Short:   "Zero-knowledge encrypted .env cloud storage",
	Version: normalizeVersion(version),
}

var newAPIClient = api.New

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetVersionTemplate(FormatVersion(versionInfo()))
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(swapCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(envCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(versionCmd)
}

func apiBaseURL() string {
	if value := os.Getenv("OBE_API_URL"); value != "" {
		return value
	}
	return defaultAPIURL
}

func loadClient() (*api.Client, error) {
	credentials, err := loadCredentials()
	if err != nil {
		return nil, err
	}
	baseURL := apiBaseURL()
	if os.Getenv("OBE_API_URL") == "" && credentials.APIURL != "" {
		baseURL = credentials.APIURL
	}
	return newAPIClient(baseURL, credentials.Token), nil
}

func loadProjectConfig() (*ProjectConfig, error) {
	data, err := os.ReadFile(projectConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s not found; run: obe init", projectConfigFile)
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

func saveProjectConfig(config ProjectConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(projectConfigFile, data, 0600)
}

func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".obe", "credentials.json"), nil
}

func loadCredentials() (*Credentials, error) {
	path, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("credentials not found; run: obe login")
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

func saveCredentials(credentials Credentials) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

func resolveEnvironment(flagValue string, config *ProjectConfig) string {
	if flagValue != "" {
		return flagValue
	}
	if config.ActiveEnvironment != "" {
		return config.ActiveEnvironment
	}
	return "development"
}
