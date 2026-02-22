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
	fmt.Fprintln(os.Stderr, "gc: not yet implemented")
	return 1
}
