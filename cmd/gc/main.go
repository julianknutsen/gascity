// gc is the Gas City CLI — an orchestration-builder for multi-agent workflows.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the gc CLI with the given args, writing output to stdout and
// errors to stderr. Returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc: no command specified") //nolint:errcheck // best-effort stderr
		return 1
	}

	switch args[0] {
	case "start":
		return cmdStart(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc: unknown command %q\n", args[0]) //nolint:errcheck // best-effort stderr
		return 1
	}
}

func cmdStart(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc start: missing city path") //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout

	w("Welcome to Gas City!")
	w("To configure your new city, add a `city.toml` file.")
	w("")
	w("To get started with one of the built-in configurations, use `gc init`.")
	w("")
	w("To add a rig (project), use `gc rig add <path>`.")
	w("")
	w("For help, use `gc help`.")
	return 0
}
