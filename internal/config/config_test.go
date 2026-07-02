package config

import (
	"errors"
	"strings"
	"testing"
)

func TestParseValidConfiguration(t *testing.T) {
	cfg, err := Parse([]byte(validYAML()))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("version = %d, want 1", cfg.Version)
	}
	if cfg.Project.Environment != "esp32-s3-devkitc-1" {
		t.Fatalf("environment = %q", cfg.Project.Environment)
	}
	if cfg.Device.Baud != 115200 {
		t.Fatalf("baud = %d, want 115200", cfg.Device.Baud)
	}
	if len(cfg.Steps) != 3 {
		t.Fatalf("steps length = %d, want 3", len(cfg.Steps))
	}
	if len(cfg.Assertions) != 4 {
		t.Fatalf("assertions length = %d, want 4", len(cfg.Assertions))
	}
	if cfg.Assertions[3].CaseSensitive {
		t.Fatalf("case_sensitive override was not applied")
	}
}

func TestParseInvalidSchemaVersion(t *testing.T) {
	_, err := Parse([]byte(strings.Replace(validYAML(), "version: 1", "version: 2", 1)))
	if err == nil {
		t.Fatal("expected error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if validationErr.Path != "version" {
		t.Fatalf("path = %q, want version", validationErr.Path)
	}
}

func TestParseUnknownField(t *testing.T) {
	input := strings.Replace(validYAML(), "name: esp32-bno055-smoke-test", "name: esp32-bno055-smoke-test\nextra: nope", 1)
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if validationErr.Path != "extra" {
		t.Fatalf("path = %q, want extra", validationErr.Path)
	}
	if validationErr.Line == 0 {
		t.Fatalf("expected line number")
	}
}

func TestParseUnknownStep(t *testing.T) {
	input := strings.Replace(validYAML(), "  - upload", "  - flash", 1)
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported step") {
		t.Fatalf("error = %q, want unsupported step", err.Error())
	}
}

func validYAML() string {
	return `version: 1
name: esp32-bno055-smoke-test
project:
  directory: .
  environment: esp32-s3-devkitc-1
device:
  platform: esp32
  port: auto
  baud: 115200
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
  - serial_contains:
      value: "BNO055 detected"
      within: 10s
  - serial_not_contains:
      value: "Guru Meditation Error"
  - serial_not_contains:
      value: "panic"
      case_sensitive: false
`
}
