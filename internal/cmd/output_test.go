package cmd

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

var ansiSequence = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainOutput(value string) string {
	return ansiSequence.ReplaceAllString(value, "")
}

func TestOutputStylesEmitANSIWhenColorIsForced(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	var out bytes.Buffer
	styles := newOutputStyles(&out)
	rendered := styles.success.Render("linked")
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("rendered=%q, want ANSI escape sequence", rendered)
	}
}
