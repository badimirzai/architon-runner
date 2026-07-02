package serial

import (
	"context"
	"fmt"
	"time"

	seriallib "go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

type Hardware struct{}

func (Hardware) ListPorts(context.Context) ([]PortInfo, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	out := make([]PortInfo, 0, len(ports))
	for _, port := range ports {
		out = append(out, PortInfo{
			Name:         port.Name,
			Description:  port.Product,
			VendorID:     port.VID,
			ProductID:    port.PID,
			SerialNumber: port.SerialNumber,
		})
	}
	return out, nil
}

func (Hardware) Open(ctx context.Context, name string, baud int) (Stream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	mode := &seriallib.Mode{BaudRate: baud}
	port, err := seriallib.Open(name, mode)
	if err != nil {
		return nil, err
	}
	return &livePort{Port: port}, nil
}

type livePort struct {
	seriallib.Port
}

func (p *livePort) Reset(ctx context.Context) error {
	type modemControl interface {
		SetDTR(bool) error
		SetRTS(bool) error
	}
	control, ok := p.Port.(modemControl)
	if !ok {
		return fmt.Errorf("serial port does not support DTR/RTS reset")
	}
	if err := control.SetDTR(false); err != nil {
		return err
	}
	if err := control.SetRTS(true); err != nil {
		return err
	}
	if err := sleep(ctx, 100*time.Millisecond); err != nil {
		return err
	}
	if err := control.SetRTS(false); err != nil {
		return err
	}
	if err := control.SetDTR(true); err != nil {
		return err
	}
	return sleep(ctx, 100*time.Millisecond)
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
