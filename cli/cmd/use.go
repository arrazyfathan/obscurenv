package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var useKey string
var usePushCurrent bool
var useFile string

var useCmd = &cobra.Command{
	Use:   "use [environment]",
	Short: "Switch active environment and pull its local env file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		return runUse(cmd, target)
	},
}

var useListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List remote environments available for use",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		environments, err := listRemoteEnvironments(config.ProjectSlug)
		if err != nil {
			return err
		}
		for _, environment := range environments {
			fmt.Fprintln(cmd.OutOrStdout(), environment)
		}
		return nil
	},
}

func runUse(cmd *cobra.Command, target string) error {
	config, err := loadProjectConfig()
	if err != nil {
		return err
	}
	file, err := resolveManagedFile(useFile, config, usePushCurrent)
	if err != nil {
		return err
	}
	current := config.ActiveEnvironment
	if target == "" {
		target, err = chooseRemoteEnvironment(cmd, config.ProjectSlug, current)
		if err != nil {
			return err
		}
	}
	passphrase, err := promptSecret("Encryption passphrase", useKey)
	if err != nil {
		return err
	}
	info(cmd.OutOrStdout(), fmt.Sprintf("Switching environment to %q...", target))
	if usePushCurrent {
		if err := withSpinner(cmd.OutOrStdout(), "Pushing "+current, func() error {
			_, perr := pushEnvironment(current, passphrase, file)
			return perr
		}); err != nil {
			return fmt.Errorf("push current environment %q: %w", current, err)
		}
	}
	config.ActiveEnvironment = target
	if err := saveProjectConfig(*config); err != nil {
		return err
	}
	if err := withSpinner(cmd.OutOrStdout(), "Pulling "+target, func() error {
		_, perr := pullEnvironment(target, passphrase, file, true)
		return perr
	}); err != nil {
		config.ActiveEnvironment = current
		_ = saveProjectConfig(*config)
		return fmt.Errorf("pull target environment %q: %w", target, err)
	}
	success(cmd.OutOrStdout(), fmt.Sprintf("Local %s now uses %q.", file, target))
	return nil
}

func chooseRemoteEnvironment(cmd *cobra.Command, projectSlug, current string) (string, error) {
	environments, err := listRemoteEnvironments(projectSlug)
	if err != nil {
		return "", err
	}
	if len(environments) == 0 {
		return "", fmt.Errorf("no remote environments found for project %q", projectSlug)
	}
	return chooseEnvironmentFromList(cmd, environments, current)
}

func listRemoteEnvironments(projectSlug string) ([]string, error) {
	client, err := loadClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.List(projectSlug)
	if err != nil {
		return nil, err
	}
	return resp.Environments, nil
}

func resolveEnvironmentChoice(value string, environments []string) (string, error) {
	for i, environment := range environments {
		if value == environment || value == fmt.Sprint(i+1) {
			return environment, nil
		}
	}
	return "", fmt.Errorf("unknown environment %q", value)
}

func init() {
	useCmd.AddCommand(useListCmd)
	useCmd.Flags().StringVarP(&useKey, "key", "k", "", "Encryption passphrase")
	useCmd.Flags().StringVar(&useFile, "file", "", "Managed local file path, such as .env or local.properties")
	useCmd.Flags().BoolVar(&usePushCurrent, "push-current", false, "Push the current local env file before switching")
}
