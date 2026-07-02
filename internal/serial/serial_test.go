package serial

import (
	"context"
	"errors"
	"testing"
)

type detectorFunc func(context.Context) ([]PortInfo, error)

func (f detectorFunc) ListPorts(ctx context.Context) ([]PortInfo, error) {
	return f(ctx)
}

func TestSelectPortExplicit(t *testing.T) {
	port, _, err := SelectPort(context.Background(), detectorFunc(func(context.Context) ([]PortInfo, error) {
		t.Fatal("detector should not be called for explicit port")
		return nil, nil
	}), "/dev/cu.usbmodem1101")
	if err != nil {
		t.Fatalf("SelectPort returned error: %v", err)
	}
	if port != "/dev/cu.usbmodem1101" {
		t.Fatalf("port = %q", port)
	}
}

func TestSelectPortAmbiguous(t *testing.T) {
	_, candidates, err := SelectPort(context.Background(), detectorFunc(func(context.Context) ([]PortInfo, error) {
		return []PortInfo{
			{Name: "/dev/cu.usbmodem1101"},
			{Name: "/dev/cu.usbserial-0001"},
		}, nil
	}), "auto")
	if err == nil {
		t.Fatal("expected error")
	}
	var ambiguous *AmbiguousPortError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("error type = %T, want AmbiguousPortError", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(candidates))
	}
}
