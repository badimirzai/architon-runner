package serial

import (
	"context"
	"fmt"
	"strings"
)

type PortInfo struct {
	Name         string
	Description  string
	VendorID     string
	ProductID    string
	SerialNumber string
}

type Detector interface {
	ListPorts(ctx context.Context) ([]PortInfo, error)
}

type Opener interface {
	Open(ctx context.Context, name string, baud int) (Stream, error)
}

type Stream interface {
	Read(p []byte) (int, error)
	Close() error
}

type Resetter interface {
	Reset(ctx context.Context) error
}

type AmbiguousPortError struct {
	Candidates []PortInfo
}

func (e *AmbiguousPortError) Error() string {
	names := make([]string, 0, len(e.Candidates))
	for _, candidate := range e.Candidates {
		names = append(names, candidate.Name)
	}
	return fmt.Sprintf("multiple plausible serial ports found: %s", strings.Join(names, ", "))
}

func SelectPort(ctx context.Context, detector Detector, configured string) (string, []PortInfo, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" && configured != "auto" {
		return configured, nil, nil
	}
	ports, err := detector.ListPorts(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("list serial ports: %w", err)
	}
	candidates := make([]PortInfo, 0, len(ports))
	for _, port := range ports {
		if plausible(port) {
			candidates = append(candidates, port)
		}
	}
	switch len(candidates) {
	case 0:
		return "", candidates, fmt.Errorf("no plausible ESP32 serial port found; connect a board or configure device.port")
	case 1:
		return candidates[0].Name, candidates, nil
	default:
		return "", candidates, &AmbiguousPortError{Candidates: candidates}
	}
}

func plausible(port PortInfo) bool {
	text := strings.ToLower(strings.Join([]string{
		port.Name,
		port.Description,
		port.VendorID,
		port.ProductID,
		port.SerialNumber,
	}, " "))
	if strings.Contains(text, "bluetooth") {
		return false
	}
	markers := []string{
		"usbmodem",
		"usbserial",
		"ttyusb",
		"ttyacm",
		"cu.usb",
		"/dev/serial/by-id",
		"cp210",
		"ch340",
		"wch",
		"silicon labs",
		"ftdi",
		"jtag/serial",
		"esp32",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return port.VendorID != "" || port.ProductID != ""
}
