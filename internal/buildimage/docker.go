package buildimage

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Build runs `docker build` on the given context directory.
func Build(ctx context.Context, contextDir, tag string, stdout, stderr io.Writer) error {
	args := []string{"build", "-t", tag, contextDir}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	return nil
}

// Push runs `docker push` for the given tag.
func Push(ctx context.Context, tag string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "push", tag)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker push: %w", err)
	}
	return nil
}
