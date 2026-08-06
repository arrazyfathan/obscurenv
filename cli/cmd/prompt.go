package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func promptString(label, fallback string) (string, error) {
	if interactive() {
		value := fallback
		input := huh.NewInput().Title(label).Value(&value)
		if fallback != "" {
			input = input.Placeholder(fallback)
		}
		if err := huh.NewForm(huh.NewGroup(input)).Run(); err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			value = fallback
		}
		return value, nil
	}
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
	if interactive() {
		secret := ""
		input := huh.NewInput().Title(label).EchoMode(huh.EchoModePassword).Value(&secret)
		if err := huh.NewForm(huh.NewGroup(input)).Run(); err != nil {
			return "", err
		}
		value = strings.TrimSpace(secret)
		if value == "" {
			return "", fmt.Errorf("%s is required", strings.ToLower(label))
		}
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
	if interactive() {
		value := fallback
		confirm := huh.NewConfirm().
			Title(label).
			Inline(true).
			Affirmative("Yes").
			Negative("No").
			Value(&value)
		if err := huh.NewForm(huh.NewGroup(confirm)).Run(); err != nil {
			return false, err
		}
		return value, nil
	}
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

func interactiveSelect(label string, options []string, current string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options available")
	}
	choice := current
	opts := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		opts = append(opts, huh.NewOption(o, o))
	}
	height := len(options)
	if height > 8 {
		height = 8
	}
	selectField := huh.NewSelect[string]().
		Title(label).
		Options(opts...).
		Height(height).
		Value(&choice)
	if err := huh.NewForm(huh.NewGroup(selectField)).Run(); err != nil {
		return "", err
	}
	if choice == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return choice, nil
}
