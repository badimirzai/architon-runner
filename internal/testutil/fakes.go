package testutil

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/badimirzai/architon-runner/internal/platformio"
	serialport "github.com/badimirzai/architon-runner/internal/serial"
)

type ProcessResponse struct {
	Result platformio.Result
	Err    error
}

type FakeProcessRunner struct {
	LookPathErr error
	Responses   []ProcessResponse
	Calls       []platformio.Request
}

func (f *FakeProcessRunner) LookPath(file string) (string, error) {
	if f.LookPathErr != nil {
		return "", f.LookPathErr
	}
	return "/usr/local/bin/" + file, nil
}

func (f *FakeProcessRunner) Run(ctx context.Context, req platformio.Request) (platformio.Result, error) {
	f.Calls = append(f.Calls, req)
	select {
	case <-ctx.Done():
		return platformio.Result{
			Command:   append([]string{req.Name}, req.Args...),
			StartedAt: time.Now().UTC(),
			ExitCode:  -1,
		}, ctx.Err()
	default:
	}
	response := ProcessResponse{
		Result: platformio.Result{
			Command:   append([]string{req.Name}, req.Args...),
			StartedAt: time.Now().UTC(),
			Duration:  time.Millisecond,
			ExitCode:  0,
			Stdout:    "ok\n",
		},
	}
	if len(f.Responses) > 0 {
		response = f.Responses[0]
		f.Responses = f.Responses[1:]
	}
	if len(response.Result.Command) == 0 {
		response.Result.Command = append([]string{req.Name}, req.Args...)
	}
	if response.Result.StartedAt.IsZero() {
		response.Result.StartedAt = time.Now().UTC()
	}
	if response.Result.Duration == 0 {
		response.Result.Duration = time.Millisecond
	}
	if req.Stdout != nil && response.Result.Stdout != "" {
		_, _ = io.WriteString(req.Stdout, response.Result.Stdout)
	}
	if req.Stderr != nil && response.Result.Stderr != "" {
		_, _ = io.WriteString(req.Stderr, response.Result.Stderr)
	}
	return response.Result, response.Err
}

type FakeDetector struct {
	Ports []serialport.PortInfo
	Err   error
}

func (f FakeDetector) ListPorts(context.Context) ([]serialport.PortInfo, error) {
	return append([]serialport.PortInfo(nil), f.Ports...), f.Err
}

type OpenCall struct {
	Name string
	Baud int
}

type FakeOpener struct {
	Streams []serialport.Stream
	Errs    []error
	Calls   []OpenCall
}

func (f *FakeOpener) Open(ctx context.Context, name string, baud int) (serialport.Stream, error) {
	f.Calls = append(f.Calls, OpenCall{Name: name, Baud: baud})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if len(f.Errs) > 0 {
		err := f.Errs[0]
		f.Errs = f.Errs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(f.Streams) == 0 {
		return nil, fmt.Errorf("no fake serial stream configured")
	}
	stream := f.Streams[0]
	f.Streams = f.Streams[1:]
	return stream, nil
}

type SerialEvent struct {
	After time.Duration
	Data  string
	Err   error
}

type ScriptedSerial struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	closed chan struct{}
	once   sync.Once
}

func NewScriptedSerial(events ...SerialEvent) *ScriptedSerial {
	reader, writer := io.Pipe()
	stream := &ScriptedSerial{
		reader: reader,
		writer: writer,
		closed: make(chan struct{}),
	}
	go stream.play(events)
	return stream
}

func (s *ScriptedSerial) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *ScriptedSerial) Close() error {
	s.once.Do(func() {
		close(s.closed)
		_ = s.reader.Close()
		_ = s.writer.Close()
	})
	return nil
}

func (s *ScriptedSerial) play(events []SerialEvent) {
	for _, event := range events {
		if event.After > 0 {
			timer := time.NewTimer(event.After)
			select {
			case <-s.closed:
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if event.Data != "" {
			if _, err := io.WriteString(s.writer, event.Data); err != nil {
				return
			}
		}
		if event.Err != nil {
			_ = s.writer.CloseWithError(event.Err)
			return
		}
	}
	<-s.closed
}

type ResettableSerial struct {
	*ScriptedSerial
	ResetCalled bool
}

func NewResettableSerial(events ...SerialEvent) *ResettableSerial {
	return &ResettableSerial{ScriptedSerial: NewScriptedSerial(events...)}
}

func (s *ResettableSerial) Reset(context.Context) error {
	s.ResetCalled = true
	return nil
}

type StepClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewStepClock(start time.Time) *StepClock {
	return &StepClock{now: start}
}

func (c *StepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Millisecond)
	return c.now
}

func (c *StepClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
