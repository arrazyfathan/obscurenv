package cmd

import (
	"path/filepath"
	"strings"
)

var localOnlyPropertyKeys = map[string]bool{
	"sdk.dir": true,
}

func isLocalPropertiesFile(path string) bool {
	return filepath.Base(path) == gradleEnvFile
}

func propertyKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed[0] == '#' || trimmed[0] == '!' {
		return ""
	}
	if i := strings.IndexAny(trimmed, "=:"); i > 0 {
		return strings.TrimSpace(trimmed[:i])
	}
	return ""
}

func stripLocalOnlyProperties(content []byte) []byte {
	lines := splitPropertyLines(content)
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if !localOnlyPropertyKeys[propertyKey(line)] {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		return []byte{}
	}
	result := strings.Join(kept, "\n")
	if hasTrailingNewline(content) {
		result += "\n"
	}
	return []byte(result)
}

func mergeLocalOnlyProperties(pulled, existing []byte) []byte {
	localLines := localOnlyPropertyLines(existing)
	if len(localLines) == 0 {
		return pulled
	}

	lines := splitPropertyLines(pulled)
	filtered := make([]string, 0, len(lines)+len(localLines))
	for _, line := range lines {
		if !localOnlyPropertyKeys[propertyKey(line)] {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 1 && filtered[0] == "" {
		filtered = nil
	}
	filtered = append(filtered, localLines...)

	result := strings.Join(filtered, "\n")
	if hasTrailingNewline(pulled) {
		result += "\n"
	}
	return []byte(result)
}

func localOnlyPropertyLines(content []byte) []string {
	var out []string
	for _, line := range splitPropertyLines(content) {
		if localOnlyPropertyKeys[propertyKey(line)] {
			out = append(out, line)
		}
	}
	return out
}

func splitPropertyLines(content []byte) []string {
	return strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
}

func hasTrailingNewline(content []byte) bool {
	return len(content) > 0 && content[len(content)-1] == '\n'
}
