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
	Name           string
	image          string
	NetworkName    string
	IPv4           string
	entrypointBin  string
	entrypointArgs []string
	workdir        string
	env            map[string]string
	// whether to delete container after run()
	cleanup bool
	// map of source -> dest paths to mount
	volumes          map[string]string
	// port mappings in format "host:container"
	ports            []string
	cFlag            bool
	detached         bool
	log              logr.Logger
	containerToolBin string
	reproducerCmd    *string
}

type Option func(c *container)

func WithImage(i string) Option {
	return func(c *container) {
		c.image = i
	}
}

func WithName(n string) Option {
	return func(c *container) {
		c.Name = n
	}
}

func WithNetwork(w string) Option {
	return func(c *container) {
		c.NetworkName = w
	}
}

func WithIPv4(ip string) Option {
	return func(c *container) {
		c.IPv4 = ip
	}
}

func WithEntrypointBin(b string) Option {
	return func(c *container) {
		c.entrypointBin = b
	}
}

func WithContainerToolBin(r string) Option {
	return func(c *container) {
		c.containerToolBin = r
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

func WithDetachedMode(d bool) Option {
	return func(c *container) {
		c.detached = d
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

// WithProxy adds proxy environment variables to the container
func WithPortPublish(ports ...string) Option {
	return func(c *container) {
		c.ports = ports
	}
}

func WithProxy(httpProxy, httpsProxy, noProxy string) Option {
	return func(c *container) {
		// Pass proxy environment variables from host to container
		proxyVars := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy"}
		for _, proxyVar := range proxyVars {
			if value := os.Getenv(proxyVar); value != "" {
				c.env[proxyVar] = value
			}
		}

		// Pass proxy settings from command line flags to container
		proxyFlags := map[string]string{
			"HTTP_PROXY":  httpProxy,
			"HTTPS_PROXY": httpsProxy,
			"NO_PROXY":    noProxy,
		}

		for envVar, value := range proxyFlags {
			if value != "" {
				c.env[envVar] = value
			}
		}
	}
}

func RandomName() string {
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
		image:            "",
		containerToolBin: "podman",
		entrypointArgs:   []string{},
		volumes:          make(map[string]string),
		stdout:           []io.Writer{os.Stdout},
		env:              map[string]string{},
		stderr:           []io.Writer{os.Stderr},
		Name:             "",
		NetworkName:      "",
		// by default, remove the container after run()
		cleanup:  true,
		cFlag:    false,
		detached: false,
		log:      logr.Discard(),
	}
}

func (c *container) Run(ctx context.Context, opts ...Option) error {
	var err error
	for _, opt := range opts {
		opt(c)
	}
	if c.image == "" || c.containerToolBin == "" {
		return fmt.Errorf("image and containerToolBin must be set")
	}
	args := []string{"run"}
	os := runtime.GOOS
	if c.detached {
		args = append(args, "-d")
	}
	if c.cleanup {
		args = append(args, "--rm")
	}
	if c.Name != "" {
		args = append(args, "--name")
		args = append(args, c.Name)
	} else {
		args = append(args, "--name")
		args = append(args, RandomName())
	}
	if c.NetworkName != "" {
		args = append(args, "--network")
		args = append(args, c.NetworkName)
	}
	if c.IPv4 != "" {
		args = append(args, "--ip")
		args = append(args, c.IPv4)
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
			args = append(args, fmt.Sprintf("%s:%s:z",
				filepath.Clean(sourcePath), path.Clean(destPath)))
		} else {
			args = append(args, fmt.Sprintf("%s:%s",
				filepath.Clean(sourcePath), path.Clean(destPath)))
		}
	}
	for _, portMapping := range c.ports {
		args = append(args, "-p")
		args = append(args, portMapping)
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
			c.containerToolBin, reproducer)
	}
	cmd := exec.CommandContext(ctx, c.containerToolBin, args...)
	fmt.Printf("%v", cmd.String())
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
	c.log.Info("executing command",
		"container tool", c.containerToolBin, "cmd", c.entrypointBin, "args", strings.Join(args, " "))
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
		c.containerToolBin,
		"rm", c.Name)
	c.log.Info("removing container",
		"container tool", c.containerToolBin, "name", c.Name)
	return cmd.Run()
}

// RunCommand performs whatever is sent to it in the command argument
func (c *container) RunCommand(ctx context.Context, logger logr.Logger, command ...string) error {
	cmd := exec.CommandContext(ctx, c.containerToolBin, command...)
	errBytes := &bytes.Buffer{}
	logger.Info("executing command", "container tool", c.containerToolBin, "cmd", c.entrypointBin, "args", strings.Join(command, " "))
	output, err := cmd.CombinedOutput()
	fmt.Printf("\n%v", cmd.String())
	fmt.Printf("\n%s", string(output))
	if err != nil {
		logger.Error(err, "container run error during cleanup")
		if _, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf(errBytes.String())
		}
		return err
	}
	return nil
}
