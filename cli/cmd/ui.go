package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var noColorFlag bool

var (
	greenStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	cyanStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	yellowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	redStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
)

func colorEnabled(w io.Writer) bool {
	if noColorFlag || os.Getenv("NO_COLOR") != "" || os.Getenv("OBE_NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func interactive() bool {
	if noColorFlag || os.Getenv("NO_COLOR") != "" || os.Getenv("OBE_NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func success(w io.Writer, msg string) {
	if colorEnabled(w) {
		fmt.Fprintf(w, "%s %s\n", greenStyle.Render("✓"), msg)
		return
	}
	fmt.Fprintln(w, msg)
}

func info(w io.Writer, msg string) {
	if colorEnabled(w) {
		fmt.Fprintf(w, "%s %s\n", cyanStyle.Render("ℹ"), msg)
		return
	}
	fmt.Fprintln(w, msg)
}

func warn(w io.Writer, msg string) {
	if colorEnabled(w) {
		fmt.Fprintf(w, "%s %s\n", yellowStyle.Render("!"), msg)
		return
	}
	fmt.Fprintln(w, msg)
}

func fail(w io.Writer, msg string) {
	if colorEnabled(w) {
		fmt.Fprintf(w, "%s %s\n", redStyle.Render("✗"), msg)
		return
	}
	fmt.Fprintln(w, msg)
}

func dim(w io.Writer, msg string) {
	if colorEnabled(w) {
		fmt.Fprintln(w, dimStyle.Render(msg))
		return
	}
	fmt.Fprintln(w, msg)
}

func accent(w io.Writer, msg string) {
	if colorEnabled(w) {
		fmt.Fprintln(w, accentStyle.Render(msg))
		return
	}
	fmt.Fprintln(w, msg)
}

func printTable(w io.Writer, headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i := range headers {
			if i < len(row) && len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}
	line := func(cells []string) string {
		var sb strings.Builder
		for i := range headers {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			if i < len(headers)-1 {
				sb.WriteString(cell)
				sb.WriteString(strings.Repeat(" ", widths[i]-len(cell)+2))
			} else {
				sb.WriteString(cell)
			}
		}
		return sb.String()
	}
	fmt.Fprintln(w, line(headers))
	for _, row := range rows {
		fmt.Fprintln(w, line(row))
	}
}

type spinnerModel struct {
	spinner spinner.Model
	label   string
}

func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m spinnerModel) View() string {
	return accentStyle.Render(m.spinner.View()) + " " + m.label
}

func withSpinner(w io.Writer, label string, fn func() error) error {
	if !interactive() {
		return fn()
	}
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = accentStyle
	p := tea.NewProgram(spinnerModel{spinner: s, label: label})
	result := make(chan error, 1)
	go func() {
		result <- fn()
		p.Send(tea.Quit())
	}()
	p.Run()
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(w, "\r\x1b[K")
	}
	return <-result
}
