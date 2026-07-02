package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/badimirzai/architon-runner/internal/config"
	"github.com/badimirzai/architon-runner/internal/exitcode"
	"github.com/badimirzai/architon-runner/internal/platformio"
	"github.com/badimirzai/architon-runner/internal/runner"
	serialport "github.com/badimirzai/architon-runner/internal/serial"
)

func main() {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintln(os.Stderr, "architon: internal error")
			os.Exit(exitcode.EnvironmentFailure)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		usage(stdout)
		return exitcode.InvalidConfig
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "architon %s\n", runner.Version)
		return exitcode.OK
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "test":
		return runTest(ctx, args[1:], stdout, stderr, getenv)
	case "-h", "--help", "help":
		usage(stdout)
		return exitcode.OK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return exitcode.InvalidConfig
	}
}

func runInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", config.DefaultFilename, "test file to create")
	name := flags.String("name", "", "test name")
	environment := flags.String("environment", "", "PlatformIO environment")
	port := flags.String("port", "auto", "serial port or auto")
	baud := flags.Int("baud", config.DefaultBaud, "serial baud rate")
	force := flags.Bool("force", false, "overwrite an existing test file")
	if err := flags.Parse(args); err != nil {
		return exitcode.InvalidConfig
	}
	if strings.TrimSpace(*file) == "" {
		fmt.Fprintln(stderr, "invalid configuration: --file must not be empty")
		return exitcode.InvalidConfig
	}
	if !*force {
		if _, err := os.Stat(*file); err == nil {
			fmt.Fprintf(stderr, "%s already exists; use --force to overwrite it\n", *file)
			return exitcode.InvalidConfig
		} else if !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "cannot inspect %s: %v\n", *file, err)
			return exitcode.EnvironmentFailure
		}
	}

	testName := strings.TrimSpace(*name)
	if testName == "" {
		testName = defaultTestName()
	}
	pioEnvironment := strings.TrimSpace(*environment)
	if pioEnvironment == "" {
		pioEnvironment = detectPlatformIOEnvironment("platformio.ini")
	}
	if pioEnvironment == "" {
		pioEnvironment = "esp32-s3-devkitc-1"
	}
	content := starterYAML(testName, pioEnvironment, strings.TrimSpace(*port), *baud)
	if err := os.WriteFile(*file, []byte(content), 0o644); err != nil {
		fmt.Fprintf(stderr, "cannot write %s: %v\n", *file, err)
		return exitcode.EnvironmentFailure
	}
	fmt.Fprintf(stdout, "Created %s\n", *file)
	fmt.Fprintln(stdout, "Edit assertions to match the text your firmware prints, then run: architon validate && architon test --verbose")
	return exitcode.OK
}

func runValidate(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", config.DefaultFilename, "test file")
	if err := flags.Parse(args); err != nil {
		return exitcode.InvalidConfig
	}
	cfg, err := loadAndValidate(*file, "")
	if err != nil {
		fmt.Fprintf(stderr, "invalid configuration: %v\n", err)
		return exitcode.InvalidConfig
	}
	fmt.Fprintf(stdout, "Configuration valid: %s\n", cfg.SourcePath)
	return exitcode.OK
}

func runTest(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", config.DefaultFilename, "test file")
	port := flags.String("port", "", "serial port override")
	verbose := flags.Bool("verbose", false, "stream PlatformIO output")
	noColor := flags.Bool("no-color", false, "disable colored output")
	if err := flags.Parse(args); err != nil {
		return exitcode.InvalidConfig
	}
	cfg, err := loadAndValidate(*file, *port)
	if err != nil {
		fmt.Fprintf(stderr, "invalid configuration: %v\n", err)
		return exitcode.InvalidConfig
	}
	color := !*noColor && getenv("NO_COLOR") == ""
	app := runner.New(runner.Dependencies{
		PlatformIO: platformio.New(nil),
		Detector:   serialport.Hardware{},
		Opener:     serialport.Hardware{},
	})
	_, code, err := app.Run(ctx, cfg, runner.Options{
		Verbose: *verbose,
		Color:   color,
		Stdout:  stdout,
		Stderr:  stderr,
	})
	if errors.Is(err, context.Canceled) {
		return exitcode.Interrupted
	}
	return code
}

func loadAndValidate(file, portOverride string) (config.Config, error) {
	cfg, err := config.Load(file)
	if err != nil {
		return config.Config{}, err
	}
	cfg = config.ApplyPortOverride(cfg, portOverride)
	if err := config.ValidateProjectDirectory(cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func defaultTestName() string {
	wd, err := os.Getwd()
	if err != nil {
		return "esp32-smoke-test"
	}
	base := strings.TrimSpace(filepath.Base(wd))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "esp32-smoke-test"
	}
	base = strings.ToLower(base)
	replacer := strings.NewReplacer(" ", "-", "_", "-", ".", "-", "/", "-", "\\", "-")
	return replacer.Replace(base) + "-smoke-test"
}

func detectPlatformIOEnvironment(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[env:") && strings.HasSuffix(line, "]") {
			env := strings.TrimSuffix(strings.TrimPrefix(line, "[env:"), "]")
			return strings.TrimSpace(env)
		}
	}
	return ""
}

func starterYAML(name, environment, port string, baud int) string {
	if port == "" {
		port = "auto"
	}
	if baud <= 0 {
		baud = config.DefaultBaud
	}
	return fmt.Sprintf(`version: 1
name: %s
project:
  directory: .
  environment: %s
device:
  platform: esp32
  port: %s
  baud: %d
execution:
  build_timeout: 120s
  upload_timeout: 60s
  boot_timeout: 15s
  test_timeout: 30s
steps:
  - build
  - upload
  - monitor:
      duration: 10s
assertions:
  - serial_contains:
      value: "System ready"
      within: 10s
  - serial_not_contains:
      value: "panic"
      case_sensitive: false
`, yamlString(name), yamlString(environment), yamlString(port), baud)
}

func yamlString(value string) string {
	return strconv.Quote(value)
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  architon version")
	fmt.Fprintln(w, "  architon init [--file architon.test.yaml] [--environment esp32-s3-devkitc-1] [--port auto] [--force]")
	fmt.Fprintln(w, "  architon validate [--file architon.test.yaml]")
	fmt.Fprintln(w, "  architon test [--file architon.test.yaml] [--port /dev/cu.usbmodem1101] [--verbose] [--no-color]")
}
