package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/obscurenv/obscurenv/cli/pkg/api"
	"github.com/spf13/cobra"
)

const (
	projectConfigFile = ".obe.json"
	defaultAPIURL     = "https://localhost:8080"
	defaultEnvFile    = ".env"
	gradleEnvFile     = "local.properties"
)

type ProjectConfig struct {
	ProjectSlug       string `json:"project_slug"`
	ActiveEnvironment string `json:"active_environment"`
	EnvFile           string `json:"env_file,omitempty"`
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
	rootCmd.AddCommand(useCmd)
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

func rememberManagedFile(config *ProjectConfig, file string) error {
	file, err := validateManagedFile(file)
	if err != nil {
		return err
	}
	if config.EnvFile == file {
		return nil
	}
	config.EnvFile = file
	return saveProjectConfig(*config)
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

func resolveManagedFile(flagValue string, config *ProjectConfig, requireExisting bool) (string, error) {
	if flagValue != "" {
		return validateManagedFile(flagValue)
	}
	if config != nil && config.EnvFile != "" {
		return validateManagedFile(config.EnvFile)
	}

	dotenvExists, err := fileExists(defaultEnvFile)
	if err != nil {
		return "", err
	}
	gradleExists, err := fileExists(gradleEnvFile)
	if err != nil {
		return "", err
	}

	switch {
	case dotenvExists && gradleExists:
		return "", fmt.Errorf("both %s and %s exist; choose one with --file", defaultEnvFile, gradleEnvFile)
	case gradleExists:
		return gradleEnvFile, nil
	case dotenvExists:
		return defaultEnvFile, nil
	case requireExisting:
		return "", fmt.Errorf("no managed file found; create %s or %s, or choose one with --file", defaultEnvFile, gradleEnvFile)
	default:
		return defaultEnvFile, nil
	}
}

func validateManagedFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("file path must be relative to the project directory")
	}
	if pathContainsParent(path) {
		return "", fmt.Errorf("file path must not contain ..")
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return "", fmt.Errorf("file path is required")
	}
	return clean, nil
}

func pathContainsParent(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return !info.IsDir(), nil
}
