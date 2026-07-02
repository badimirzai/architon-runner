package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/badimirzai/architon-runner/internal/config"
	"github.com/badimirzai/architon-runner/internal/exitcode"
	"github.com/badimirzai/architon-runner/internal/platformio"
	"github.com/badimirzai/architon-runner/internal/report"
	serialport "github.com/badimirzai/architon-runner/internal/serial"
	"github.com/badimirzai/architon-runner/internal/testutil"
)

func TestRunSuccessfulEndToEndWithFakes(t *testing.T) {
	cfg := testConfig(t)
	process := &testutil.FakeProcessRunner{}
	opener := &testutil.FakeOpener{Streams: []serialport.Stream{
		testutil.NewScriptedSerial(
			testutil.SerialEvent{After: time.Millisecond, Data: "System ready\n"},
			testutil.SerialEvent{After: time.Millisecond, Data: "BNO055 detected\n"},
		),
	}}
	result, code := runWithFakes(t, cfg, process, onePortDetector(), opener, context.Background())
	if code != exitcode.OK {
		t.Fatalf("exit code = %d, want 0; result=%+v", code, result)
	}
	if len(process.Calls) != 2 {
		t.Fatalf("process calls = %d, want 2", len(process.Calls))
	}
	if got := process.Calls[1].Args; !equalStrings(got, []string{"run", "-e", "esp32-s3-devkitc-1", "-t", "upload", "--upload-port", "/dev/cu.usbmodem1101"}) {
		t.Fatalf("upload args = %#v", got)
	}
	if len(result.Assertions) != 4 {
		t.Fatalf("assertions = %d, want 4", len(result.Assertions))
	}
	for _, assertion := range result.Assertions {
		if assertion.Status != "passed" {
			t.Fatalf("assertion failed unexpectedly: %+v", assertion)
		}
	}
	assertArtifactExists(t, result.ArtifactPaths["config"])
	assertArtifactExists(t, result.ArtifactPaths["serial_log"])
	assertArtifactExists(t, result.ArtifactPaths["build_stdout"])
	assertArtifactExists(t, result.ArtifactPaths["build_stderr"])
	assertArtifactExists(t, result.ArtifactPaths["upload_stdout"])
	assertArtifactExists(t, result.ArtifactPaths["upload_stderr"])
	assertArtifactExists(t, result.ArtifactPaths["result"])

	data, err := os.ReadFile(result.ArtifactPaths["result"])
	if err != nil {
		t.Fatalf("read result json: %v", err)
	}
	var decoded report.RunResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode result json: %v", err)
	}
	if decoded.ExitCode != exitcode.OK || decoded.FinalStatus != "passed" {
		t.Fatalf("decoded result = %+v", decoded)
	}
}

func TestRunBuildFailure(t *testing.T) {
	cfg := testConfig(t)
	process := &testutil.FakeProcessRunner{Responses: []testutil.ProcessResponse{{
		Result: platformio.Result{ExitCode: 1, Stderr: "compile error\n"},
	}}}
	result, code := runWithFakes(t, cfg, process, onePortDetector(), &testutil.FakeOpener{}, context.Background())
	if code != exitcode.ExecutionFailure {
		t.Fatalf("exit code = %d, want %d", code, exitcode.ExecutionFailure)
	}
	if len(result.Steps) != 1 || result.Steps[0].Name != "build" {
		t.Fatalf("steps = %+v", result.Steps)
	}
	data, err := os.ReadFile(result.ArtifactPaths["build_stderr"])
	if err != nil {
		t.Fatalf("read build stderr: %v", err)
	}
	if !bytes.Contains(data, []byte("compile error")) {
		t.Fatalf("build stderr did not contain failure output: %q", string(data))
	}
}

func TestRunUploadFailure(t *testing.T) {
	cfg := testConfig(t)
	process := &testutil.FakeProcessRunner{Responses: []testutil.ProcessResponse{
		{Result: platformio.Result{ExitCode: 0, Stdout: "built\n"}},
		{Result: platformio.Result{ExitCode: 1, Stderr: "upload failed\n"}},
	}}
	result, code := runWithFakes(t, cfg, process, onePortDetector(), &testutil.FakeOpener{}, context.Background())
	if code != exitcode.ExecutionFailure {
		t.Fatalf("exit code = %d, want %d", code, exitcode.ExecutionFailure)
	}
	if len(result.Steps) != 2 || result.Steps[1].Name != "upload" {
		t.Fatalf("steps = %+v", result.Steps)
	}
}

func TestRunSerialPortAmbiguity(t *testing.T) {
	cfg := testConfig(t)
	process := &testutil.FakeProcessRunner{}
	detector := testutil.FakeDetector{Ports: []serialport.PortInfo{
		{Name: "/dev/cu.usbmodem1101"},
		{Name: "/dev/cu.usbserial-0001"},
	}}
	result, code := runWithFakes(t, cfg, process, detector, &testutil.FakeOpener{}, context.Background())
	if code != exitcode.SerialFailure {
		t.Fatalf("exit code = %d, want %d", code, exitcode.SerialFailure)
	}
	if result.SerialPort != "" {
		t.Fatalf("serial port = %q, want empty", result.SerialPort)
	}
}

func TestRunMissingExpectedOutput(t *testing.T) {
	cfg := testConfig(t)
	process := &testutil.FakeProcessRunner{}
	opener := &testutil.FakeOpener{Streams: []serialport.Stream{
		testutil.NewScriptedSerial(testutil.SerialEvent{After: time.Millisecond, Data: "System ready\n"}),
	}}
	result, code := runWithFakes(t, cfg, process, onePortDetector(), opener, context.Background())
	if code != exitcode.AssertionFailed {
		t.Fatalf("exit code = %d, want %d", code, exitcode.AssertionFailed)
	}
	if assertionFailures(result) != 1 {
		t.Fatalf("assertion failures = %d, want 1; assertions=%+v", assertionFailures(result), result.Assertions)
	}
}

func TestRunForbiddenOutput(t *testing.T) {
	cfg := testConfig(t)
	process := &testutil.FakeProcessRunner{}
	opener := &testutil.FakeOpener{Streams: []serialport.Stream{
		testutil.NewScriptedSerial(
			testutil.SerialEvent{After: time.Millisecond, Data: "System ready\n"},
			testutil.SerialEvent{After: time.Millisecond, Data: "BNO055 detected\n"},
			testutil.SerialEvent{After: time.Millisecond, Data: "PANIC\n"},
		),
	}}
	result, code := runWithFakes(t, cfg, process, onePortDetector(), opener, context.Background())
	if code != exitcode.AssertionFailed {
		t.Fatalf("exit code = %d, want %d", code, exitcode.AssertionFailed)
	}
	if assertionFailures(result) != 1 {
		t.Fatalf("assertion failures = %d, want 1; assertions=%+v", assertionFailures(result), result.Assertions)
	}
	var forbidden report.AssertionResult
	for _, assertion := range result.Assertions {
		if assertion.Status == "failed" && assertion.Value == "panic" {
			forbidden = assertion
		}
	}
	if forbidden.Evidence == "" {
		t.Fatalf("forbidden assertion should include evidence: %+v", result.Assertions)
	}
}

func TestRunAssertionTimeout(t *testing.T) {
	cfg := testConfig(t)
	cfg.Assertions = cfg.Assertions[:1]
	cfg.Assertions[0].Within = time.Millisecond
	process := &testutil.FakeProcessRunner{}
	opener := &testutil.FakeOpener{Streams: []serialport.Stream{
		testutil.NewScriptedSerial(testutil.SerialEvent{After: 10 * time.Millisecond, Data: "System ready\n"}),
	}}
	result, code := runWithFakesClock(t, cfg, process, onePortDetector(), opener, context.Background(), RealClock{})
	if code != exitcode.AssertionFailed {
		t.Fatalf("exit code = %d, want %d", code, exitcode.AssertionFailed)
	}
	if assertionFailures(result) != 1 {
		t.Fatalf("assertion failures = %d, want 1; assertions=%+v", assertionFailures(result), result.Assertions)
	}
}

func TestRunCancellation(t *testing.T) {
	cfg := testConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, code := runWithFakes(t, cfg, &testutil.FakeProcessRunner{}, onePortDetector(), &testutil.FakeOpener{}, ctx)
	if code != exitcode.Interrupted {
		t.Fatalf("exit code = %d, want %d", code, exitcode.Interrupted)
	}
	if result.FinalStatus != "interrupted" {
		t.Fatalf("status = %q, want interrupted", result.FinalStatus)
	}
}

func runWithFakes(t *testing.T, cfg config.Config, process *testutil.FakeProcessRunner, detector testutil.FakeDetector, opener *testutil.FakeOpener, ctx context.Context) (report.RunResult, int) {
	t.Helper()
	return runWithFakesClock(t, cfg, process, detector, opener, ctx, testutil.NewStepClock(time.Date(2026, 7, 2, 21, 14, 5, 0, time.UTC)))
}

func runWithFakesClock(t *testing.T, cfg config.Config, process *testutil.FakeProcessRunner, detector testutil.FakeDetector, opener *testutil.FakeOpener, ctx context.Context, clock Clock) (report.RunResult, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := New(Dependencies{
		PlatformIO: platformio.New(process),
		Detector:   detector,
		Opener:     opener,
		Clock:      clock,
	})
	result, code, err := app.Run(ctx, cfg, Options{
		ArtifactRoot: filepath.Join(t.TempDir(), ".architon", "runs"),
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if code == exitcode.OK && err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	return result, code
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	projectDir := t.TempDir()
	input := `version: 1
name: esp32-bno055-smoke-test
project:
  directory: .
  environment: esp32-s3-devkitc-1
device:
  platform: esp32
  port: auto
  baud: 115200
execution:
  build_timeout: 1s
  upload_timeout: 1s
  boot_timeout: 5ms
  test_timeout: 20ms
steps:
  - build
  - upload
  - monitor:
      duration: 20ms
assertions:
  - serial_contains:
      value: "System ready"
      within: 20ms
  - serial_contains:
      value: "BNO055 detected"
      within: 20ms
  - serial_not_contains:
      value: "Guru Meditation Error"
  - serial_not_contains:
      value: "panic"
      case_sensitive: false
`
	cfg, err := config.Parse([]byte(input))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	cfg.Project.Directory = projectDir
	return cfg
}

func onePortDetector() testutil.FakeDetector {
	return testutil.FakeDetector{Ports: []serialport.PortInfo{{Name: "/dev/cu.usbmodem1101"}}}
}

func assertArtifactExists(t *testing.T, path string) {
	t.Helper()
	if path == "" {
		t.Fatal("empty artifact path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("artifact %s missing: %v", path, err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
