package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func promptString(label, fallback string) (string, error) {
	if !stdinIsTerminal() {
		if fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	if fallback != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", label, fallback)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", label)
	}
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	return value, nil
}

func promptRequired(label, value, fallback string) (string, error) {
	if value != "" {
		return value, nil
	}
	value, err := promptString(label, fallback)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}

func promptSecret(label, value string) (string, error) {
	if value != "" {
		return value, nil
	}
	if !stdinIsTerminal() {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	fmt.Fprintf(os.Stderr, "%s: ", label)
	data, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}

func promptConfirm(label string, fallback bool) (bool, error) {
	if !stdinIsTerminal() {
		return fallback, nil
	}
	suffix := "Y/n"
	if !fallback {
		suffix = "y/N"
	}
	value, err := promptString(label+" ["+suffix+"]", "")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(value) {
	case "":
		return fallback, nil
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("answer must be yes or no")
	}
}
