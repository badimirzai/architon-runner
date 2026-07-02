package report

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type Console struct {
	Out   io.Writer
	Err   io.Writer
	Color bool
}

func (c Console) Header() {
	fmt.Fprintln(c.out(), "Architon Hardware Test")
}

func (c Console) Pass(label, detail string) {
	fmt.Fprintf(c.out(), "%s %-40s %s\n", c.green("✓"), label, detail)
}

func (c Console) Fail(label, detail string) {
	fmt.Fprintf(c.out(), "%s %-40s %s\n", c.red("✗"), label, detail)
}

func (c Console) Info(message string) {
	fmt.Fprintln(c.out(), message)
}

func (c Console) Final(status string, assertionFailures int, artifacts string) {
	if status == "passed" {
		fmt.Fprintln(c.out(), c.green("PASSED"))
	} else {
		fmt.Fprintln(c.out(), c.red("FAILED"))
	}
	if assertionFailures == 1 {
		fmt.Fprintln(c.out(), "1 assertion failed")
	} else if assertionFailures > 1 {
		fmt.Fprintf(c.out(), "%d assertions failed\n", assertionFailures)
	}
	if artifacts != "" {
		fmt.Fprintf(c.out(), "Artifacts: %s\n", artifacts)
	}
}

func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func (c Console) out() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	return io.Discard
}

func (c Console) green(s string) string {
	if !c.Color {
		return s
	}
	return "\x1b[32m" + s + "\x1b[0m"
}

func (c Console) red(s string) string {
	if !c.Color {
		return s
	}
	return "\x1b[31m" + s + "\x1b[0m"
}

func CleanDetail(parts ...string) string {
	return strings.TrimSpace(strings.Join(parts, " "))
}
