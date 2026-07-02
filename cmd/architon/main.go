package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
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

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  architon version")
	fmt.Fprintln(w, "  architon validate [--file architon.test.yaml]")
	fmt.Fprintln(w, "  architon test [--file architon.test.yaml] [--port /dev/cu.usbmodem1101] [--verbose] [--no-color]")
}
