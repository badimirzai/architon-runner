package platformio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type ProcessRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, req Request) (Result, error)
}

type Request struct {
	Name    string
	Args    []string
	Dir     string
	Timeout time.Duration
	Stdout  io.Writer
	Stderr  io.Writer
}

type Result struct {
	Command   []string
	StartedAt time.Time
	Duration  time.Duration
	ExitCode  int
	Stdout    string
	Stderr    string
}

type Service struct {
	runner ProcessRunner
	binary string
}

func New(runner ProcessRunner) *Service {
	if runner == nil {
		runner = OSRunner{}
	}
	return &Service{runner: runner}
}

func (s *Service) Verify() error {
	path, err := s.runner.LookPath("pio")
	if err != nil {
		return fmt.Errorf("PlatformIO CLI not found: install PlatformIO Core, or add the VS Code PlatformIO Core path to PATH; Architon checks ~/.platformio/penv/bin automatically")
	}
	s.binary = path
	return nil
}

func (s *Service) Build(ctx context.Context, projectDir, environment string, timeout time.Duration, stdout, stderr io.Writer) (Result, error) {
	return s.runner.Run(ctx, Request{
		Name:    s.commandName(),
		Args:    []string{"run", "-e", environment},
		Dir:     projectDir,
		Timeout: timeout,
		Stdout:  stdout,
		Stderr:  stderr,
	})
}

func (s *Service) Upload(ctx context.Context, projectDir, environment, port string, timeout time.Duration, stdout, stderr io.Writer) (Result, error) {
	return s.runner.Run(ctx, Request{
		Name:    s.commandName(),
		Args:    []string{"run", "-e", environment, "-t", "upload", "--upload-port", port},
		Dir:     projectDir,
		Timeout: timeout,
		Stdout:  stdout,
		Stderr:  stderr,
	})
}

func (s *Service) commandName() string {
	if s.binary != "" {
		return s.binary
	}
	path, err := s.runner.LookPath("pio")
	if err == nil {
		s.binary = path
		return path
	}
	return "pio"
}

type OSRunner struct{}

func (OSRunner) LookPath(file string) (string, error) {
	if path, err := exec.LookPath(file); err == nil {
		return path, nil
	}
	if file != "pio" {
		return "", exec.ErrNotFound
	}
	for _, candidate := range platformIOCandidates() {
		if isExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func (OSRunner) Run(ctx context.Context, req Request) (Result, error) {
	if req.Name == "" {
		return Result{}, errors.New("command name is required")
	}
	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	started := time.Now().UTC()
	cmd := exec.CommandContext(runCtx, req.Name, req.Args...)
	cmd.Dir = req.Dir

	var stdout, stderr bytes.Buffer
	if req.Stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, req.Stdout)
	} else {
		cmd.Stdout = &stdout
	}
	if req.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, req.Stderr)
	} else {
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	result := Result{
		Command:   append([]string{req.Name}, req.Args...),
		StartedAt: started,
		Duration:  time.Since(started),
		ExitCode:  0,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
	}
	if runCtx.Err() != nil {
		result.ExitCode = -1
		return result, runCtx.Err()
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	result.ExitCode = -1
	return result, err
}

func platformIOCandidates() []string {
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".platformio", "penv", "bin", "pio"),
			filepath.Join(home, ".platformio", "penv", "bin", "platformio"),
		)
	}
	candidates = append(candidates,
		"/opt/homebrew/bin/pio",
		"/usr/local/bin/pio",
	)
	return candidates
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}
