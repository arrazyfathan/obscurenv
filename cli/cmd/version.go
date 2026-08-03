package cmd

import (
	"fmt"
	"regexp"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
	commit  = "unknown"
	builtAt = "unknown"
)

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type VersionInfo struct {
	Version string
	Commit  string
	BuiltAt string
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(cmd.OutOrStdout(), FormatVersion(versionInfo()))
	},
}

func versionInfo() VersionInfo {
	return VersionInfo{
		Version: normalizeVersion(version),
		Commit:  commit,
		BuiltAt: builtAt,
	}
}

func normalizeVersion(value string) string {
	if value == "" {
		return "0.0.0-dev"
	}
	if len(value) > 0 && value[0] == 'v' {
		value = value[1:]
	}
	if !semverPattern.MatchString(value) {
		return "0.0.0-dev"
	}
	return value
}

func FormatVersion(info VersionInfo) string {
	return fmt.Sprintf("obe version %s\ncommit: %s\nbuilt: %s\n", info.Version, info.Commit, info.BuiltAt)
}
