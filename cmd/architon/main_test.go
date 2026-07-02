package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/badimirzai/architon-runner/internal/exitcode"
)

func TestInitCreatesStarterConfigFromPlatformIOEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("platformio.ini", []byte("[env:esp32-s3-devkitc-1]\nplatform = espressif32\n"), 0o644); err != nil {
		t.Fatalf("write platformio.ini: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"init"}, &stdout, &stderr, os.Getenv)
	if code != exitcode.OK {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, exitcode.OK, stderr.String())
	}

	data, err := os.ReadFile("architon.test.yaml")
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		`environment: "esp32-s3-devkitc-1"`,
		`port: "auto"`,
		`value: "System ready"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated config missing %q:\n%s", want, text)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"validate"}, &stdout, &stderr, os.Getenv)
	if code != exitcode.OK {
		t.Fatalf("validate exit code = %d, want %d; stderr=%s", code, exitcode.OK, stderr.String())
	}
}

func TestInitRefusesToOverwriteExistingConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("architon.test.yaml", []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"init"}, &stdout, &stderr, os.Getenv)
	if code != exitcode.InvalidConfig {
		t.Fatalf("exit code = %d, want %d", code, exitcode.InvalidConfig)
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("stderr = %q, want already exists message", stderr.String())
	}
}
