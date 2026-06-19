package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type outputStyles struct {
	section lipgloss.Style
	header  lipgloss.Style
	cell    lipgloss.Style
	muted   lipgloss.Style
	info    lipgloss.Style
	success lipgloss.Style
	warn    lipgloss.Style
	danger  lipgloss.Style
	accent  lipgloss.Style
}

func newOutputStyles(out io.Writer) outputStyles {
	r := lipgloss.NewRenderer(out)
	return outputStyles{
		section: r.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED")),
		header:  r.NewStyle().Bold(true).Foreground(lipgloss.Color("#6B7280")),
		cell:    r.NewStyle(),
		muted:   r.NewStyle().Foreground(lipgloss.Color("#6B7280")),
		info:    r.NewStyle().Foreground(lipgloss.Color("#2563EB")),
		success: r.NewStyle().Foreground(lipgloss.Color("#16A34A")),
		warn:    r.NewStyle().Foreground(lipgloss.Color("#D97706")),
		danger:  r.NewStyle().Foreground(lipgloss.Color("#DC2626")),
		accent:  r.NewStyle().Foreground(lipgloss.Color("#7C3AED")),
	}
}

func renderSectionTable(out io.Writer, styles outputStyles, title string, headers []string, rows [][]string, styleFunc func(row, col int, value string) lipgloss.Style) {
	_, _ = fmt.Fprintln(out, styles.section.Render(title))
	if len(rows) == 0 {
		_, _ = fmt.Fprintf(out, "  %s\n", styles.muted.Render("none"))
		return
	}

	t := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		BorderRow(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return styles.header.PaddingRight(2)
			}
			if styleFunc != nil && row >= 0 && row < len(rows) && col >= 0 && col < len(rows[row]) {
				return styleFunc(row, col, rows[row][col]).PaddingRight(2)
			}
			return styles.cell.PaddingRight(2)
		})
	_, _ = fmt.Fprintln(out, strings.TrimRight(t.Render(), " "))
}

func boolText(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func selectionText(values []string, empty string) string {
	if len(values) == 0 {
		return empty
	}
	return strings.Join(values, ", ")
}

func dashIfEmpty(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func boolStyle(styles outputStyles, value string) lipgloss.Style {
	if value == "yes" {
		return styles.success
	}
	return styles.muted
}

func existsStyle(styles outputStyles, value string) lipgloss.Style {
	if value == "yes" {
		return styles.success
	}
	return styles.warn
}
