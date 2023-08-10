package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type containerCommand struct {
	stdout         io.Writer
	stderr         io.Writer
	containerName  string
	containerImage string
	entrypointBin  string
	entrypointArgs []string
	workdir        string
	// map of source -> dest paths to mount
	volumes map[string]string
}

type Option func(c *containerCommand)

func WithContainerImage(i string) Option {
	return func(c *containerCommand) {
		c.containerImage = i
	}
}

func WithContainerName(n string) Option {
	return func(c *containerCommand) {
		c.containerName = n
	}
}

func WithEntrypointBin(b string) Option {
	return func(c *containerCommand) {
		c.entrypointBin = b
	}
}

func WithEntrypointArgs(args ...string) Option {
	return func(c *containerCommand) {
		c.entrypointArgs = args
	}
}

func WithWorkDir(w string) Option {
	return func(c *containerCommand) {
		c.workdir = w
	}
}

func WithVolumes(m map[string]string) Option {
	return func(c *containerCommand) {
		c.volumes = m
	}
}

func WithStdout(o io.Writer) Option {
	return func(c *containerCommand) {
		c.stdout = o
	}
}

func WithStderr(e io.Writer) Option {
	return func(c *containerCommand) {
		c.stderr = e
	}
}

func NewContainerCommand(ctx context.Context, opts ...Option) *exec.Cmd {
	c := &containerCommand{
		containerImage: Settings.RunnerImage,
		entrypointArgs: []string{},
		volumes:        make(map[string]string),
		stdout:         os.Stdout,
		stderr:         os.Stderr,
	}

	for _, opt := range opts {
		opt(c)
	}

	args := []string{"run", "-it"}
	if c.containerName != "" {
		args = append(args, "--name")
		args = append(args, c.containerName)
	}

	if c.entrypointBin != "" {
		args = append(args, "--entrypoint")
		args = append(args, c.entrypointBin)
	}

	if c.workdir != "" {
		args = append(args, "--workdir")
		args = append(args, c.workdir)
	}

	for sourcePath, destPath := range c.volumes {
		args = append(args, "-v")
		args = append(args, fmt.Sprintf("%s:%s:Z", sourcePath, destPath))
	}

	args = append(args, c.containerImage)
	if len(c.entrypointArgs) > 0 {
		args = append(args, c.entrypointArgs...)
	}

	cmd := exec.CommandContext(ctx, Settings.PodmanBinary, args...)
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	return cmd
}
