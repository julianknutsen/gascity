// gc is the Gas City CLI — an orchestration-builder for multi-agent workflows.
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(main1())
}

func main1() int {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "gc: no command specified")
		return 1
	}

	switch os.Args[1] {
	case "start":
		return cmdStart(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "gc: unknown command %q\n", os.Args[1])
		return 1
	}
}

func cmdStart(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "gc start: missing city path")
		return 1
	}

	fmt.Println("Welcome to Gas City!")
	fmt.Println("To configure your new city, add a `city.toml` file.")
	fmt.Println()
	fmt.Println("To get started with one of the built-in configurations, use `gc init`.")
	fmt.Println()
	fmt.Println("To add a rig (project), use `gc rig add <path>`.")
	fmt.Println()
	fmt.Println("For help, use `gc help`.")
	return 0
}
