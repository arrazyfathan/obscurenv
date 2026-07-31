package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var runKey string
var runEnv string

var runCmd = &cobra.Command{
	Use:   "run -- <command>",
	Short: "Run a command with cloud env vars injected in memory",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadProjectConfig()
		if err != nil {
			return err
		}
		environment := resolveEnvironment(runEnv, config)
		passphrase, err := promptSecret("Encryption passphrase", runKey)
		if err != nil {
			return err
		}
		plaintext, err := pullEnvironment(environment, passphrase, false)
		if err != nil {
			return err
		}
		envVars, err := parseEnv(string(plaintext))
		if err != nil {
			return err
		}
		child := exec.Command(args[0], args[1:]...)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		child.Stdin = os.Stdin
		child.Env = append(os.Environ(), envVars...)
		return child.Run()
	},
}

func init() {
	runCmd.Flags().StringVarP(&runKey, "key", "k", "", "Encryption passphrase")
	runCmd.Flags().StringVarP(&runEnv, "env", "e", "", "Environment name")
}

func parseEnv(content string) ([]string, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	values := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid env line: %q", scanner.Text())
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		values = append(values, key+"="+value)
	}
	return values, scanner.Err()
}
