package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-logr/logr"
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
	volumes             map[string]string
	cFlag               bool
	log                 logr.Logger
	containerRuntimeBin string
	reproducerCmd       *string
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

func WithcFlag(cl bool) Option {
	return func(c *container) {
		c.cFlag = cl
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

func WithLog(l logr.Logger) Option {
	return func(c *container) {
		c.log = l
	}
}

func WithReproduceCmd(r *string) Option {
	return func(c *container) {
		c.reproducerCmd = r
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
		image:               "",
		containerRuntimeBin: "podman",
		entrypointArgs:      []string{},
		volumes:             make(map[string]string),
		stdout:              []io.Writer{os.Stdout},
		env:                 map[string]string{},
		stderr:              []io.Writer{os.Stderr},
		name:                randomName(),
		// by default, remove the container after run()
		cleanup: true,
		cFlag:   false,
		log:     logr.Discard(),
	}
}

func (c *container) Run(ctx context.Context, opts ...Option) error {
	var err error
	for _, opt := range opts {
		opt(c)
	}
	if c.image == "" || c.containerRuntimeBin == "" {
		return fmt.Errorf("image and containerRuntimeBin must be set")
	}
	args := []string{"run"}
	os := runtime.GOOS
	if c.cleanup {
		args = append(args, "--rm")
	}
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
		if os == "linux" {
			args = append(args, fmt.Sprintf("%s:%s:Z",
				filepath.Clean(sourcePath), path.Clean(destPath)))
		} else {
			args = append(args, fmt.Sprintf("%s:%s",
				filepath.Clean(sourcePath), path.Clean(destPath)))
		}
	}
	for k, v := range c.env {
		args = append(args, "--env")
		args = append(args, fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, c.image)
	if c.cFlag {
		args = append(args, "-c")
	}
	if len(c.entrypointArgs) > 0 {
		args = append(args, c.entrypointArgs...)
	}
	if c.reproducerCmd != nil {
		reproducer := strings.ReplaceAll(strings.Join(args, " "), " --rm", "")
		*c.reproducerCmd = fmt.Sprintf("%s %s",
			c.containerRuntimeBin, reproducer)
	}
	cmd := exec.CommandContext(ctx, c.containerRuntimeBin, args...)
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
	c.log.Info("executing podman command",
		"podman", c.containerRuntimeBin, "cmd", c.entrypointBin, "args", strings.Join(args, " "))
	err = cmd.Run()
	if err != nil {
		c.log.Error(err, "container run error")
		if _, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf(errBytes.String())
		}
		return err
	}
	return nil
}

func (c *container) Rm(ctx context.Context) error {
	cmd := exec.CommandContext(
		ctx,
		c.containerRuntimeBin,
		"rm", c.name)
	c.log.Info("removing container",
		"podman", c.containerRuntimeBin, "name", c.name)
	return cmd.Run()
}
