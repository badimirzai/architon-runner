package platformio

import (
	"context"
	"io"
	"testing"
)

type fakeRunner struct {
	path string
	reqs []Request
}

func (f *fakeRunner) LookPath(string) (string, error) {
	return f.path, nil
}

func (f *fakeRunner) Run(_ context.Context, req Request) (Result, error) {
	f.reqs = append(f.reqs, req)
	return Result{Command: append([]string{req.Name}, req.Args...)}, nil
}

func TestServiceUsesResolvedPlatformIOPath(t *testing.T) {
	runner := &fakeRunner{path: "/Users/example/.platformio/penv/bin/pio"}
	service := New(runner)
	if err := service.Verify(); err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}

	if _, err := service.Build(context.Background(), ".", "esp32-s3-devkitc-1", 0, io.Discard, io.Discard); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(runner.reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(runner.reqs))
	}
	if runner.reqs[0].Name != runner.path {
		t.Fatalf("command name = %q, want %q", runner.reqs[0].Name, runner.path)
	}
}
