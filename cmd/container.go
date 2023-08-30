package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type container struct {
	stdout         []io.Writer
	stderr         []io.Writer
	name           string
	image          string
	entrypointBin  string
	entrypointArgs []string
	workdir        string
	env            map[string]string
	// whether to delete container after run()
	cleanup bool
	// map of source -> dest paths to mount
	volumes map[string]string
}

type Option func(c *container)

func WithImage(i string) Option {
	return func(c *container) {
		c.image = i
	}
}

func WithName(n string) Option {
	return func(c *container) {
		c.name = n
	}
}

func WithEntrypointBin(b string) Option {
	return func(c *container) {
		c.entrypointBin = b
	}
}

func WithEntrypointArgs(args ...string) Option {
	return func(c *container) {
		c.entrypointArgs = args
	}
}

func WithWorkDir(w string) Option {
	return func(c *container) {
		c.workdir = w
	}
}

func WithVolumes(m map[string]string) Option {
	return func(c *container) {
		c.volumes = m
	}
}

func WithStdout(o ...io.Writer) Option {
	return func(c *container) {
		c.stdout = o
	}
}

func WithStderr(e ...io.Writer) Option {
	return func(c *container) {
		c.stderr = e
	}
}

func WithCleanup(cl bool) Option {
	return func(c *container) {
		c.cleanup = cl
	}
}

func WithEnv(k string, v string) Option {
	return func(c *container) {
		c.env[k] = v
	}
}

func randomName() string {
	rand.Seed(int64(time.Now().Nanosecond()))
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func NewContainer() *container {
	return &container{
		image:          Settings.RunnerImage,
		entrypointArgs: []string{},
		volumes:        make(map[string]string),
		stdout:         []io.Writer{os.Stdout},
		env:            map[string]string{},
		stderr:         []io.Writer{os.Stderr},
		name:           randomName(),
		// by default, remove the container after run()
		cleanup: true,
	}
}

func (c *container) Exists(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx,
		Settings.PodmanBinary,
		"ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed checking status of container %s", c.name)
	}
	for _, found := range strings.Split(string(output), "\n") {
		if found == c.name {
			return true, nil
		}
	}
	return false, nil
}

func (c *container) Run(ctx context.Context, opts ...Option) error {
	var err error
	for _, opt := range opts {
		opt(c)
	}
	exists, err := c.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check status of container %s", c.name)
	}
	if exists {
		return fmt.Errorf("container %s already exists, must remove existing before running", c.name)
	}
	if c.cleanup {
		defer func() {
			if rmErr := c.Rm(ctx); rmErr != nil {
				err = rmErr
			}
		}()
	}
	args := []string{"run"}
	os := runtime.GOOS
	if c.name != "" {
		args = append(args, "--name")
		args = append(args, c.name)
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
		// TODO: check this on windows
		if os == "linux" {
			args = append(args, fmt.Sprintf("%s:%s:Z",
				filepath.Clean(sourcePath), filepath.Clean(destPath)))
		} else {
			args = append(args, fmt.Sprintf("%s:%s",
				filepath.Clean(sourcePath), filepath.Clean(destPath)))
		}
	}
	for k, v := range c.env {
		args = append(args, "--env")
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, c.image)
	if len(c.entrypointArgs) > 0 {
		args = append(args, c.entrypointArgs...)
	}
	cmd := exec.CommandContext(ctx, Settings.PodmanBinary, args...)
	errBytes := &bytes.Buffer{}
	cmd.Stdout = nil
	cmd.Stderr = errBytes
	if c.stdout != nil {
		cmd.Stdout = io.MultiWriter(c.stdout...)
	}
	if c.stderr != nil {
		cmd.Stderr = io.MultiWriter(
			append(c.stderr, errBytes)...)
	}
	err = cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf(errBytes.String())
		}
		return err
	}
	return nil
}

func (c *container) Cp(ctx context.Context, src string, dest string) error {
	if src == "" || dest == "" {
		return fmt.Errorf("source or dest cannot be empty")
	}
	exists, err := c.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check status of container %s", c.name)
	}
	if !exists {
		return fmt.Errorf("container %s does not exist, cannot copy from non-existing container", c.name)
	}
	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"cp", fmt.Sprintf("%s:%s", c.name, src), dest)
	return cmd.Run()
}

func (c *container) Rm(ctx context.Context) error {
	exists, err := c.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check status of container %s", c.name)
	}
	if !exists {
		return nil
	}
	cmd := exec.CommandContext(
		ctx,
		Settings.PodmanBinary,
		"rm", c.name)
	return cmd.Run()
}
