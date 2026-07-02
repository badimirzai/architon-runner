package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/badimirzai/architon-runner/internal/assertion"
	"github.com/badimirzai/architon-runner/internal/config"
	"github.com/badimirzai/architon-runner/internal/exitcode"
	"github.com/badimirzai/architon-runner/internal/platformio"
	"github.com/badimirzai/architon-runner/internal/report"
	serialport "github.com/badimirzai/architon-runner/internal/serial"
)

const Version = "0.1.0"

type Dependencies struct {
	PlatformIO *platformio.Service
	Detector   serialport.Detector
	Opener     serialport.Opener
	Clock      Clock
}

type Options struct {
	Verbose      bool
	Color        bool
	ArtifactRoot string
	Stdout       io.Writer
	Stderr       io.Writer
}

type Runner struct {
	deps Dependencies
}

func New(deps Dependencies) *Runner {
	if deps.PlatformIO == nil {
		deps.PlatformIO = platformio.New(nil)
	}
	if deps.Detector == nil {
		deps.Detector = serialport.Hardware{}
	}
	if deps.Opener == nil {
		deps.Opener = serialport.Hardware{}
	}
	if deps.Clock == nil {
		deps.Clock = RealClock{}
	}
	return &Runner{deps: deps}
}

func (r *Runner) Run(ctx context.Context, cfg config.Config, opts Options) (report.RunResult, int, error) {
	if opts.ArtifactRoot == "" {
		opts.ArtifactRoot = filepath.Join(".architon", "runs")
	}
	console := report.Console{Out: opts.Stdout, Err: opts.Stderr, Color: opts.Color}
	console.Header()

	startedAt := r.deps.Clock.Now()
	runDir, err := createRunDir(opts.ArtifactRoot, startedAt)
	if err != nil {
		console.Fail("Artifacts directory", err.Error())
		return report.RunResult{}, exitcode.EnvironmentFailure, err
	}
	artifactPaths := map[string]string{
		"config":        filepath.Join(runDir, "config.resolved.yaml"),
		"serial_log":    filepath.Join(runDir, "serial.log"),
		"build_stdout":  filepath.Join(runDir, "build.stdout.log"),
		"build_stderr":  filepath.Join(runDir, "build.stderr.log"),
		"upload_stdout": filepath.Join(runDir, "upload.stdout.log"),
		"upload_stderr": filepath.Join(runDir, "upload.stderr.log"),
		"result":        filepath.Join(runDir, "result.json"),
	}

	result := report.RunResult{
		SchemaVersion:      report.SchemaVersion,
		RunnerVersion:      Version,
		TestName:           cfg.Name,
		StartedAt:          startedAt,
		ProjectEnvironment: cfg.Project.Environment,
		ArtifactPaths:      artifactPaths,
	}

	if data, err := config.ResolvedYAML(cfg); err != nil {
		console.Fail("Configuration resolved", err.Error())
		return r.finish(console, result, "failed", exitcode.InvalidConfig, runDir, 0, err)
	} else if err := os.WriteFile(artifactPaths["config"], data, 0o644); err != nil {
		console.Fail("Configuration artifact", err.Error())
		return r.finish(console, result, "failed", exitcode.EnvironmentFailure, runDir, 0, err)
	}
	console.Pass("Configuration valid", "")

	if err := r.deps.PlatformIO.Verify(); err != nil {
		console.Fail("PlatformIO available", err.Error())
		result.Steps = append(result.Steps, failedSyntheticStep("platformio", startedAt, r.deps.Clock.Now(), err))
		return r.finish(console, result, "failed", exitcode.EnvironmentFailure, runDir, 0, err)
	}

	var selectedPort string
	for _, step := range cfg.Steps {
		if ctx.Err() != nil {
			err := ctx.Err()
			console.Fail("Interrupted", err.Error())
			return r.finish(console, result, "interrupted", exitcode.Interrupted, runDir, assertionFailures(result), err)
		}
		switch step.Type {
		case config.StepBuild:
			stepResult, code, err := r.runBuild(ctx, cfg, opts, console, artifactPaths)
			result.Steps = append(result.Steps, stepResult)
			if err != nil || code != exitcode.OK {
				return r.finish(console, result, statusForCode(code), code, runDir, assertionFailures(result), err)
			}
		case config.StepUpload:
			if selectedPort == "" {
				port, code, err := r.selectPort(ctx, cfg, console)
				if err != nil {
					result.Steps = append(result.Steps, failedSyntheticStep("serial port", r.deps.Clock.Now(), r.deps.Clock.Now(), err))
					return r.finish(console, result, statusForCode(code), code, runDir, assertionFailures(result), err)
				}
				selectedPort = port
				result.SerialPort = selectedPort
			}
			stepResult, code, err := r.runUpload(ctx, cfg, selectedPort, opts, console, artifactPaths)
			result.Steps = append(result.Steps, stepResult)
			if err != nil || code != exitcode.OK {
				return r.finish(console, result, statusForCode(code), code, runDir, assertionFailures(result), err)
			}
		case config.StepMonitor:
			if selectedPort == "" {
				port, code, err := r.selectPort(ctx, cfg, console)
				if err != nil {
					result.Steps = append(result.Steps, failedSyntheticStep("serial port", r.deps.Clock.Now(), r.deps.Clock.Now(), err))
					return r.finish(console, result, statusForCode(code), code, runDir, assertionFailures(result), err)
				}
				selectedPort = port
				result.SerialPort = selectedPort
			}
			stepResult, assertionResults, code, err := r.runMonitor(ctx, cfg, step, selectedPort, console, artifactPaths)
			result.Steps = append(result.Steps, stepResult)
			result.Assertions = append(result.Assertions, assertionResults...)
			if err != nil || code != exitcode.OK {
				return r.finish(console, result, statusForCode(code), code, runDir, assertionFailures(result), err)
			}
		}
	}

	failures := assertionFailures(result)
	if failures > 0 {
		return r.finish(console, result, "failed", exitcode.AssertionFailed, runDir, failures, nil)
	}
	return r.finish(console, result, "passed", exitcode.OK, runDir, 0, nil)
}

func (r *Runner) runBuild(ctx context.Context, cfg config.Config, opts Options, console report.Console, artifacts map[string]string) (report.StepResult, int, error) {
	start := r.deps.Clock.Now()
	stdoutFile, stderrFile, err := openCommandLogs(artifacts["build_stdout"], artifacts["build_stderr"])
	if err != nil {
		return failedSyntheticStep("build", start, r.deps.Clock.Now(), err), exitcode.EnvironmentFailure, err
	}
	defer stdoutFile.Close()
	defer stderrFile.Close()

	stdout := io.Writer(stdoutFile)
	stderr := io.Writer(stderrFile)
	if opts.Verbose {
		stdout = io.MultiWriter(stdoutFile, opts.Stdout)
		stderr = io.MultiWriter(stderrFile, opts.Stderr)
	} else {
		console.Info("Building firmware...")
	}

	cmdResult, err := r.deps.PlatformIO.Build(ctx, cfg.Project.Directory, cfg.Project.Environment, cfg.Execution.BuildTimeout, stdout, stderr)
	completed := r.deps.Clock.Now()
	step := commandStep("build", start, completed, cmdResult, artifacts["build_stdout"], artifacts["build_stderr"])
	if errors.Is(err, context.Canceled) {
		step.Status = "interrupted"
		step.Error = err.Error()
		console.Fail("Firmware built", "interrupted")
		return step, exitcode.Interrupted, err
	}
	if err != nil {
		step.Status = "failed"
		step.Error = err.Error()
		console.Fail("Firmware built", err.Error())
		return step, exitcode.ExecutionFailure, err
	}
	if cmdResult.ExitCode != 0 {
		step.Status = "failed"
		step.Error = fmt.Sprintf("pio exited with status %d", cmdResult.ExitCode)
		console.Fail("Firmware built", step.Error)
		return step, exitcode.ExecutionFailure, errors.New(step.Error)
	}
	console.Pass("Firmware built", report.FormatDuration(cmdResult.Duration))
	return step, exitcode.OK, nil
}

func (r *Runner) runUpload(ctx context.Context, cfg config.Config, port string, opts Options, console report.Console, artifacts map[string]string) (report.StepResult, int, error) {
	start := r.deps.Clock.Now()
	stdoutFile, stderrFile, err := openCommandLogs(artifacts["upload_stdout"], artifacts["upload_stderr"])
	if err != nil {
		return failedSyntheticStep("upload", start, r.deps.Clock.Now(), err), exitcode.EnvironmentFailure, err
	}
	defer stdoutFile.Close()
	defer stderrFile.Close()

	stdout := io.Writer(stdoutFile)
	stderr := io.Writer(stderrFile)
	if opts.Verbose {
		stdout = io.MultiWriter(stdoutFile, opts.Stdout)
		stderr = io.MultiWriter(stderrFile, opts.Stderr)
	} else {
		console.Info("Uploading firmware...")
	}

	cmdResult, err := r.deps.PlatformIO.Upload(ctx, cfg.Project.Directory, cfg.Project.Environment, port, cfg.Execution.UploadTimeout, stdout, stderr)
	completed := r.deps.Clock.Now()
	step := commandStep("upload", start, completed, cmdResult, artifacts["upload_stdout"], artifacts["upload_stderr"])
	if errors.Is(err, context.Canceled) {
		step.Status = "interrupted"
		step.Error = err.Error()
		console.Fail("Firmware uploaded", "interrupted")
		return step, exitcode.Interrupted, err
	}
	if err != nil {
		step.Status = "failed"
		step.Error = err.Error()
		console.Fail("Firmware uploaded", err.Error())
		return step, exitcode.ExecutionFailure, err
	}
	if cmdResult.ExitCode != 0 {
		step.Status = "failed"
		step.Error = fmt.Sprintf("pio exited with status %d", cmdResult.ExitCode)
		console.Fail("Firmware uploaded", step.Error)
		return step, exitcode.ExecutionFailure, errors.New(step.Error)
	}
	console.Pass("Firmware uploaded", report.FormatDuration(cmdResult.Duration))
	return step, exitcode.OK, nil
}

func (r *Runner) selectPort(ctx context.Context, cfg config.Config, console report.Console) (string, int, error) {
	port, candidates, err := serialport.SelectPort(ctx, r.deps.Detector, cfg.Device.Port)
	if err == nil {
		return port, exitcode.OK, nil
	}
	var ambiguous *serialport.AmbiguousPortError
	if errors.As(err, &ambiguous) {
		console.Fail("Serial port detected", "multiple candidates")
		for _, candidate := range candidates {
			detail := strings.TrimSpace(strings.Join([]string{candidate.Name, candidate.Description}, " "))
			console.Info("  " + detail)
		}
		console.Info("Provide --port or configure device.port.")
		return "", exitcode.SerialFailure, err
	}
	console.Fail("Serial port detected", err.Error())
	return "", exitcode.SerialFailure, err
}

func (r *Runner) runMonitor(ctx context.Context, cfg config.Config, step config.Step, port string, console report.Console, artifacts map[string]string) (report.StepResult, []report.AssertionResult, int, error) {
	start := r.deps.Clock.Now()
	stream, err := r.openSerialWithRetry(ctx, port, cfg.Device.Baud, cfg.Execution.BootTimeout)
	if err != nil {
		stepResult := failedSyntheticStep("monitor", start, r.deps.Clock.Now(), err)
		console.Fail("Serial monitor opened", err.Error())
		return stepResult, nil, exitcode.SerialFailure, err
	}
	defer stream.Close()
	console.Pass("Serial monitor opened", port)

	if cfg.Execution.ResetAfterUpload {
		resetter, ok := stream.(serialport.Resetter)
		if !ok {
			err := fmt.Errorf("serial port does not support reset_after_upload")
			stepResult := failedSyntheticStep("monitor", start, r.deps.Clock.Now(), err)
			console.Fail("Board reset", err.Error())
			return stepResult, nil, exitcode.SerialFailure, err
		}
		if err := resetter.Reset(ctx); err != nil {
			stepResult := failedSyntheticStep("monitor", start, r.deps.Clock.Now(), err)
			console.Fail("Board reset", err.Error())
			return stepResult, nil, exitcode.SerialFailure, err
		}
		console.Pass("Board reset", "DTR/RTS")
	}

	logFile, err := os.OpenFile(artifacts["serial_log"], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		stepResult := failedSyntheticStep("monitor", start, r.deps.Clock.Now(), err)
		console.Fail("Serial log", err.Error())
		return stepResult, nil, exitcode.EnvironmentFailure, err
	}
	defer logFile.Close()

	specs := make([]assertion.Spec, 0, len(cfg.Assertions))
	for i, assertionCfg := range cfg.Assertions {
		specs = append(specs, assertion.Spec{
			Index:         i,
			Type:          assertion.Type(assertionCfg.Type),
			Value:         assertionCfg.Value,
			Within:        assertionCfg.Within,
			CaseSensitive: assertionCfg.CaseSensitive,
		})
	}
	monitorStart := r.deps.Clock.Now()
	evaluator := assertion.NewEvaluator(specs, monitorStart, 1024*1024)

	completed, err := r.captureSerial(ctx, stream, logFile, step.MonitorDuration, evaluator, console)
	ended := r.deps.Clock.Now()
	stepResult := report.StepResult{
		Name:        "monitor",
		Status:      "passed",
		StartedAt:   start,
		CompletedAt: ended,
		Duration:    report.FormatDuration(ended.Sub(start)),
	}
	if errors.Is(err, context.Canceled) {
		stepResult.Status = "interrupted"
		stepResult.Error = err.Error()
		return stepResult, completed, exitcode.Interrupted, err
	}
	if err != nil {
		stepResult.Status = "failed"
		stepResult.Error = err.Error()
		console.Fail("Serial capture", err.Error())
		return stepResult, completed, exitcode.SerialFailure, err
	}
	console.Pass("Serial captured", report.FormatDuration(step.MonitorDuration))
	if hasAssertionFailure(completed) {
		return stepResult, completed, exitcode.AssertionFailed, nil
	}
	return stepResult, completed, exitcode.OK, nil
}

func (r *Runner) openSerialWithRetry(ctx context.Context, port string, baud int, timeout time.Duration) (serialport.Stream, error) {
	deadline := r.deps.Clock.Now().Add(timeout)
	var lastErr error
	for {
		stream, err := r.deps.Opener.Open(ctx, port, baud)
		if err == nil {
			return stream, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !r.deps.Clock.Now().Before(deadline) {
			return nil, fmt.Errorf("open serial port %s: %w", port, lastErr)
		}
		if err := r.deps.Clock.Sleep(ctx, 250*time.Millisecond); err != nil {
			return nil, err
		}
	}
}

type readEvent struct {
	data string
	err  error
}

func (r *Runner) captureSerial(ctx context.Context, stream serialport.Stream, log io.Writer, duration time.Duration, evaluator *assertion.Evaluator, console report.Console) ([]report.AssertionResult, error) {
	captureCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	events := make(chan readEvent, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stream.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				select {
				case events <- readEvent{data: chunk}:
				case <-captureCtx.Done():
					return
				}
			}
			if err != nil {
				select {
				case events <- readEvent{err: err}:
				case <-captureCtx.Done():
				}
				return
			}
		}
	}()

	var results []report.AssertionResult
	for {
		select {
		case <-captureCtx.Done():
			_ = stream.Close()
			if errors.Is(ctx.Err(), context.Canceled) {
				return results, ctx.Err()
			}
			for _, result := range evaluator.Finish(r.deps.Clock.Now()) {
				results = append(results, renderAssertionResult(result, console))
			}
			return results, nil
		case event := <-events:
			if event.data != "" {
				at := r.deps.Clock.Now()
				writeTimestamped(log, at, event.data)
				for _, result := range evaluator.Observe(event.data, at) {
					results = append(results, renderAssertionResult(result, console))
				}
			}
			if event.err != nil {
				if errors.Is(event.err, io.EOF) {
					for _, result := range evaluator.Finish(r.deps.Clock.Now()) {
						results = append(results, renderAssertionResult(result, console))
					}
					return results, nil
				}
				return results, event.err
			}
		}
	}
}

func renderAssertionResult(result assertion.Result, console report.Console) report.AssertionResult {
	status := "passed"
	if !result.Passed {
		status = "failed"
	}
	out := report.AssertionResult{
		Name:     string(result.Type),
		Status:   status,
		Value:    result.Value,
		Duration: report.FormatDuration(result.Duration),
		Message:  result.Message,
		Evidence: result.Evidence,
	}
	if result.Passed {
		console.Pass(result.Message, report.FormatDuration(result.Duration))
	} else {
		detail := ""
		if result.Evidence != "" {
			detail = "evidence: " + result.Evidence
		}
		console.Fail(result.Message, detail)
	}
	return out
}

func writeTimestamped(w io.Writer, at time.Time, data string) {
	lines := strings.SplitAfter(data, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "\n") {
			fmt.Fprintf(w, "%s %s", at.UTC().Format(time.RFC3339Nano), line)
		} else {
			fmt.Fprintf(w, "%s %s\n", at.UTC().Format(time.RFC3339Nano), line)
		}
	}
}

func openCommandLogs(stdoutPath, stderrPath string) (*os.File, *os.File, error) {
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, err
	}
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = stdoutFile.Close()
		return nil, nil, err
	}
	return stdoutFile, stderrFile, nil
}

func createRunDir(root string, startedAt time.Time) (string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	base := startedAt.UTC().Format("2006-01-02T150405Z")
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%02d", base, i)
		}
		dir := filepath.Join(root, name)
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return dir, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not create unique run directory under %s", root)
}

func commandStep(name string, startedAt, completedAt time.Time, result platformio.Result, stdoutPath, stderrPath string) report.StepResult {
	exitCode := result.ExitCode
	status := "passed"
	if exitCode != 0 {
		status = "failed"
	}
	duration := result.Duration
	if duration == 0 {
		duration = completedAt.Sub(startedAt)
	}
	return report.StepResult{
		Name:        name,
		Status:      status,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Duration:    report.FormatDuration(duration),
		Command:     result.Command,
		ExitStatus:  &exitCode,
		StdoutPath:  stdoutPath,
		StderrPath:  stderrPath,
	}
}

func failedSyntheticStep(name string, startedAt, completedAt time.Time, err error) report.StepResult {
	return report.StepResult{
		Name:        name,
		Status:      "failed",
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Duration:    report.FormatDuration(completedAt.Sub(startedAt)),
		Error:       err.Error(),
	}
}

func (r *Runner) finish(console report.Console, result report.RunResult, status string, code int, runDir string, assertionFailureCount int, err error) (report.RunResult, int, error) {
	completedAt := r.deps.Clock.Now()
	result.CompletedAt = completedAt
	result.TotalDuration = report.FormatDuration(completedAt.Sub(result.StartedAt))
	result.FinalStatus = status
	result.ExitCode = code
	if result.ArtifactPaths != nil && result.ArtifactPaths["result"] != "" {
		if writeErr := report.WriteJSON(result.ArtifactPaths["result"], result); writeErr != nil && err == nil {
			err = writeErr
			code = exitcode.EnvironmentFailure
			result.ExitCode = code
			result.FinalStatus = "failed"
		}
	}
	console.Final(result.FinalStatus, assertionFailureCount, runDir)
	return result, code, err
}

func assertionFailures(result report.RunResult) int {
	failures := 0
	for _, assertionResult := range result.Assertions {
		if assertionResult.Status == "failed" {
			failures++
		}
	}
	return failures
}

func hasAssertionFailure(results []report.AssertionResult) bool {
	for _, result := range results {
		if result.Status == "failed" {
			return true
		}
	}
	return false
}

func statusForCode(code int) string {
	if code == exitcode.OK {
		return "passed"
	}
	if code == exitcode.Interrupted {
		return "interrupted"
	}
	return "failed"
}
