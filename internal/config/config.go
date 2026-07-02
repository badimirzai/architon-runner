package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	SchemaVersion        = 1
	DefaultFilename      = "architon.test.yaml"
	DefaultBaud          = 115200
	DefaultBuildTimeout  = 120 * time.Second
	DefaultUploadTimeout = 60 * time.Second
	DefaultBootTimeout   = 15 * time.Second
	DefaultTestTimeout   = 30 * time.Second
)

type Config struct {
	Version    int
	Name       string
	Project    Project
	Device     Device
	Execution  Execution
	Steps      []Step
	Assertions []Assertion
	SourcePath string
}

type Project struct {
	Directory   string
	Environment string
}

type Device struct {
	Platform string
	Port     string
	Baud     int
}

type Execution struct {
	BuildTimeout     time.Duration
	UploadTimeout    time.Duration
	BootTimeout      time.Duration
	TestTimeout      time.Duration
	ResetAfterUpload bool
}

type StepType string

const (
	StepBuild   StepType = "build"
	StepUpload  StepType = "upload"
	StepMonitor StepType = "monitor"
)

type Step struct {
	Type            StepType
	MonitorDuration time.Duration
	Line            int
}

type AssertionType string

const (
	AssertionSerialContains    AssertionType = "serial_contains"
	AssertionSerialNotContains AssertionType = "serial_not_contains"
)

type Assertion struct {
	Type          AssertionType
	Value         string
	Within        time.Duration
	CaseSensitive bool
	Line          int
}

type ValidationError struct {
	File    string
	Path    string
	Line    int
	Message string
}

func (e *ValidationError) Error() string {
	loc := e.Path
	if loc == "" {
		loc = "document"
	}
	if e.File != "" && e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", e.File, e.Line, loc, e.Message)
	}
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s: %s", e.Line, loc, e.Message)
	}
	if e.File != "" {
		return fmt.Sprintf("%s: %s: %s", e.File, loc, e.Message)
	}
	return fmt.Sprintf("%s: %s", loc, e.Message)
}

func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultFilename
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read test file: %w", err)
	}
	cfg, err := Parse(data)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			ve.File = path
		}
		return Config{}, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("resolve test file path: %w", err)
	}
	cfg.SourcePath = absPath
	cfg.Project.Directory = resolveProjectDir(absPath, cfg.Project.Directory)
	return cfg, nil
}

func Parse(data []byte) (Config, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return Config{}, fmt.Errorf("parse yaml: %w", err)
	}
	if len(doc.Content) == 0 {
		return Config{}, validation("", doc.Line, "file is empty")
	}
	root := doc.Content[0]
	entries, err := mapping(root, "", []string{"version", "name", "project", "device", "execution", "steps", "assertions"})
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Execution: Execution{
			BuildTimeout:  DefaultBuildTimeout,
			UploadTimeout: DefaultUploadTimeout,
			BootTimeout:   DefaultBootTimeout,
			TestTimeout:   DefaultTestTimeout,
		},
	}

	if n := entries["version"]; n != nil {
		cfg.Version, err = intScalar(n, "version")
		if err != nil {
			return Config{}, err
		}
		if cfg.Version != SchemaVersion {
			return Config{}, validation("version", n.Line, fmt.Sprintf("unsupported schema version %d; supported version is %d", cfg.Version, SchemaVersion))
		}
	} else {
		return Config{}, validation("version", root.Line, "is required")
	}

	if n := entries["name"]; n != nil {
		cfg.Name, err = stringScalar(n, "name")
		if err != nil {
			return Config{}, err
		}
		if strings.TrimSpace(cfg.Name) == "" {
			return Config{}, validation("name", n.Line, "must not be empty")
		}
	} else {
		return Config{}, validation("name", root.Line, "is required")
	}

	cfg.Project, err = parseProject(entries["project"])
	if err != nil {
		return Config{}, err
	}
	cfg.Device, err = parseDevice(entries["device"])
	if err != nil {
		return Config{}, err
	}
	cfg.Execution, err = parseExecution(entries["execution"], cfg.Execution)
	if err != nil {
		return Config{}, err
	}
	cfg.Steps, err = parseSteps(entries["steps"])
	if err != nil {
		return Config{}, err
	}
	cfg.Assertions, err = parseAssertions(entries["assertions"], cfg.Execution.TestTimeout)
	if err != nil {
		return Config{}, err
	}
	if err := validateConfig(cfg, root.Line); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ValidateProjectDirectory(cfg Config) error {
	if cfg.Project.Directory == "" {
		return validation("project.directory", 0, "is required")
	}
	info, err := os.Stat(cfg.Project.Directory)
	if err != nil {
		return validation("project.directory", 0, fmt.Sprintf("cannot access %q: %v", cfg.Project.Directory, err))
	}
	if !info.IsDir() {
		return validation("project.directory", 0, fmt.Sprintf("%q is not a directory", cfg.Project.Directory))
	}
	return nil
}

func ApplyPortOverride(cfg Config, port string) Config {
	if strings.TrimSpace(port) != "" {
		cfg.Device.Port = strings.TrimSpace(port)
	}
	return cfg
}

func ResolvedYAML(cfg Config) ([]byte, error) {
	type projectYAML struct {
		Directory   string `yaml:"directory"`
		Environment string `yaml:"environment"`
	}
	type deviceYAML struct {
		Platform string `yaml:"platform"`
		Port     string `yaml:"port"`
		Baud     int    `yaml:"baud"`
	}
	type executionYAML struct {
		BuildTimeout     string `yaml:"build_timeout"`
		UploadTimeout    string `yaml:"upload_timeout"`
		BootTimeout      string `yaml:"boot_timeout"`
		TestTimeout      string `yaml:"test_timeout"`
		ResetAfterUpload bool   `yaml:"reset_after_upload,omitempty"`
	}
	out := struct {
		Version    int              `yaml:"version"`
		Name       string           `yaml:"name"`
		Project    projectYAML      `yaml:"project"`
		Device     deviceYAML       `yaml:"device"`
		Execution  executionYAML    `yaml:"execution"`
		Steps      []any            `yaml:"steps"`
		Assertions []map[string]any `yaml:"assertions"`
	}{
		Version: cfg.Version,
		Name:    cfg.Name,
		Project: projectYAML{
			Directory:   cfg.Project.Directory,
			Environment: cfg.Project.Environment,
		},
		Device: deviceYAML{
			Platform: cfg.Device.Platform,
			Port:     cfg.Device.Port,
			Baud:     cfg.Device.Baud,
		},
		Execution: executionYAML{
			BuildTimeout:     cfg.Execution.BuildTimeout.String(),
			UploadTimeout:    cfg.Execution.UploadTimeout.String(),
			BootTimeout:      cfg.Execution.BootTimeout.String(),
			TestTimeout:      cfg.Execution.TestTimeout.String(),
			ResetAfterUpload: cfg.Execution.ResetAfterUpload,
		},
	}
	for _, step := range cfg.Steps {
		if step.Type == StepMonitor {
			out.Steps = append(out.Steps, map[string]any{
				string(StepMonitor): map[string]string{"duration": step.MonitorDuration.String()},
			})
			continue
		}
		out.Steps = append(out.Steps, string(step.Type))
	}
	for _, assertion := range cfg.Assertions {
		body := map[string]any{
			"value":          assertion.Value,
			"case_sensitive": assertion.CaseSensitive,
		}
		if assertion.Type == AssertionSerialContains {
			body["within"] = assertion.Within.String()
		}
		out.Assertions = append(out.Assertions, map[string]any{string(assertion.Type): body})
	}
	return yaml.Marshal(out)
}

func parseProject(n *yaml.Node) (Project, error) {
	if n == nil {
		return Project{}, validation("project", 0, "is required")
	}
	entries, err := mapping(n, "project", []string{"directory", "environment"})
	if err != nil {
		return Project{}, err
	}
	directory, err := requiredString(entries, "project.directory")
	if err != nil {
		return Project{}, err
	}
	environment, err := requiredString(entries, "project.environment")
	if err != nil {
		return Project{}, err
	}
	return Project{Directory: directory, Environment: environment}, nil
}

func parseDevice(n *yaml.Node) (Device, error) {
	if n == nil {
		return Device{}, validation("device", 0, "is required")
	}
	entries, err := mapping(n, "device", []string{"platform", "port", "baud"})
	if err != nil {
		return Device{}, err
	}
	platform, err := requiredString(entries, "device.platform")
	if err != nil {
		return Device{}, err
	}
	if platform != "esp32" {
		return Device{}, validation("device.platform", entries["platform"].Line, "must be \"esp32\"")
	}
	port, err := requiredString(entries, "device.port")
	if err != nil {
		return Device{}, err
	}
	baud := DefaultBaud
	if n := entries["baud"]; n != nil {
		baud, err = intScalar(n, "device.baud")
		if err != nil {
			return Device{}, err
		}
		if baud <= 0 {
			return Device{}, validation("device.baud", n.Line, "must be greater than zero")
		}
	}
	return Device{Platform: platform, Port: port, Baud: baud}, nil
}

func parseExecution(n *yaml.Node, defaults Execution) (Execution, error) {
	if n == nil {
		return defaults, nil
	}
	entries, err := mapping(n, "execution", []string{"build_timeout", "upload_timeout", "boot_timeout", "test_timeout", "reset_after_upload"})
	if err != nil {
		return Execution{}, err
	}
	out := defaults
	if n := entries["build_timeout"]; n != nil {
		out.BuildTimeout, err = durationScalar(n, "execution.build_timeout")
		if err != nil {
			return Execution{}, err
		}
	}
	if n := entries["upload_timeout"]; n != nil {
		out.UploadTimeout, err = durationScalar(n, "execution.upload_timeout")
		if err != nil {
			return Execution{}, err
		}
	}
	if n := entries["boot_timeout"]; n != nil {
		out.BootTimeout, err = durationScalar(n, "execution.boot_timeout")
		if err != nil {
			return Execution{}, err
		}
	}
	if n := entries["test_timeout"]; n != nil {
		out.TestTimeout, err = durationScalar(n, "execution.test_timeout")
		if err != nil {
			return Execution{}, err
		}
	}
	if n := entries["reset_after_upload"]; n != nil {
		out.ResetAfterUpload, err = boolScalar(n, "execution.reset_after_upload")
		if err != nil {
			return Execution{}, err
		}
	}
	for path, value := range map[string]time.Duration{
		"execution.build_timeout":  out.BuildTimeout,
		"execution.upload_timeout": out.UploadTimeout,
		"execution.boot_timeout":   out.BootTimeout,
		"execution.test_timeout":   out.TestTimeout,
	} {
		if value <= 0 {
			return Execution{}, validation(path, n.Line, "must be greater than zero")
		}
	}
	return out, nil
}

func parseSteps(n *yaml.Node) ([]Step, error) {
	if n == nil {
		return nil, validation("steps", 0, "is required")
	}
	if n.Kind != yaml.SequenceNode {
		return nil, validation("steps", n.Line, "must be a sequence")
	}
	steps := make([]Step, 0, len(n.Content))
	for i, item := range n.Content {
		path := fmt.Sprintf("steps[%d]", i)
		switch item.Kind {
		case yaml.ScalarNode:
			value, err := stringScalar(item, path)
			if err != nil {
				return nil, err
			}
			if value != string(StepBuild) && value != string(StepUpload) {
				return nil, validation(path, item.Line, fmt.Sprintf("unsupported step %q", value))
			}
			steps = append(steps, Step{Type: StepType(value), Line: item.Line})
		case yaml.MappingNode:
			entries, err := mapping(item, path, []string{string(StepMonitor)})
			if err != nil {
				return nil, err
			}
			monitor := entries[string(StepMonitor)]
			if monitor == nil {
				return nil, validation(path, item.Line, "must contain exactly one supported step")
			}
			monitorEntries, err := mapping(monitor, path+".monitor", []string{"duration"})
			if err != nil {
				return nil, err
			}
			durationNode := monitorEntries["duration"]
			if durationNode == nil {
				return nil, validation(path+".monitor.duration", monitor.Line, "is required")
			}
			duration, err := durationScalar(durationNode, path+".monitor.duration")
			if err != nil {
				return nil, err
			}
			steps = append(steps, Step{Type: StepMonitor, MonitorDuration: duration, Line: item.Line})
		default:
			return nil, validation(path, item.Line, "must be a scalar step name or a step mapping")
		}
	}
	return steps, nil
}

func parseAssertions(n *yaml.Node, defaultWithin time.Duration) ([]Assertion, error) {
	if n == nil {
		return nil, validation("assertions", 0, "is required")
	}
	if n.Kind != yaml.SequenceNode {
		return nil, validation("assertions", n.Line, "must be a sequence")
	}
	assertions := make([]Assertion, 0, len(n.Content))
	for i, item := range n.Content {
		path := fmt.Sprintf("assertions[%d]", i)
		if item.Kind != yaml.MappingNode {
			return nil, validation(path, item.Line, "must be a mapping")
		}
		if len(item.Content) != 2 {
			return nil, validation(path, item.Line, "must contain exactly one assertion")
		}
		key := item.Content[0]
		value := item.Content[1]
		switch AssertionType(key.Value) {
		case AssertionSerialContains:
			assertion, err := parseSerialContains(value, path+".serial_contains", defaultWithin)
			if err != nil {
				return nil, err
			}
			assertion.Line = key.Line
			assertions = append(assertions, assertion)
		case AssertionSerialNotContains:
			assertion, err := parseSerialNotContains(value, path+".serial_not_contains")
			if err != nil {
				return nil, err
			}
			assertion.Line = key.Line
			assertions = append(assertions, assertion)
		default:
			return nil, validation(path+"."+key.Value, key.Line, fmt.Sprintf("unsupported assertion %q", key.Value))
		}
	}
	return assertions, nil
}

func parseSerialContains(n *yaml.Node, path string, defaultWithin time.Duration) (Assertion, error) {
	entries, err := mapping(n, path, []string{"value", "within", "case_sensitive"})
	if err != nil {
		return Assertion{}, err
	}
	value, err := requiredString(entries, path+".value")
	if err != nil {
		return Assertion{}, err
	}
	out := Assertion{Type: AssertionSerialContains, Value: value, Within: defaultWithin, CaseSensitive: true}
	if n := entries["within"]; n != nil {
		out.Within, err = durationScalar(n, path+".within")
		if err != nil {
			return Assertion{}, err
		}
		if out.Within <= 0 {
			return Assertion{}, validation(path+".within", n.Line, "must be greater than zero")
		}
	}
	if n := entries["case_sensitive"]; n != nil {
		out.CaseSensitive, err = boolScalar(n, path+".case_sensitive")
		if err != nil {
			return Assertion{}, err
		}
	}
	return out, nil
}

func parseSerialNotContains(n *yaml.Node, path string) (Assertion, error) {
	entries, err := mapping(n, path, []string{"value", "case_sensitive"})
	if err != nil {
		return Assertion{}, err
	}
	value, err := requiredString(entries, path+".value")
	if err != nil {
		return Assertion{}, err
	}
	out := Assertion{Type: AssertionSerialNotContains, Value: value, CaseSensitive: true}
	if n := entries["case_sensitive"]; n != nil {
		out.CaseSensitive, err = boolScalar(n, path+".case_sensitive")
		if err != nil {
			return Assertion{}, err
		}
	}
	return out, nil
}

func validateConfig(cfg Config, line int) error {
	if len(cfg.Steps) == 0 {
		return validation("steps", line, "must include at least one step")
	}
	hasMonitor := false
	for _, step := range cfg.Steps {
		if step.Type == StepMonitor {
			hasMonitor = true
		}
	}
	if len(cfg.Assertions) > 0 && !hasMonitor {
		return validation("steps", line, "must include monitor when assertions are configured")
	}
	return nil
}

func mapping(n *yaml.Node, path string, allowed []string) (map[string]*yaml.Node, error) {
	if n == nil {
		return nil, validation(path, 0, "is required")
	}
	if n.Kind != yaml.MappingNode {
		return nil, validation(path, n.Line, "must be a mapping")
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	entries := make(map[string]*yaml.Node, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i]
		value := n.Content[i+1]
		childPath := joinPath(path, key.Value)
		if _, ok := allowedSet[key.Value]; !ok {
			return nil, validation(childPath, key.Line, "unsupported field")
		}
		if _, exists := entries[key.Value]; exists {
			return nil, validation(childPath, key.Line, "duplicate field")
		}
		entries[key.Value] = value
	}
	return entries, nil
}

func requiredString(entries map[string]*yaml.Node, path string) (string, error) {
	key := path[strings.LastIndex(path, ".")+1:]
	n := entries[key]
	if n == nil {
		return "", validation(path, 0, "is required")
	}
	value, err := stringScalar(n, path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", validation(path, n.Line, "must not be empty")
	}
	return value, nil
}

func stringScalar(n *yaml.Node, path string) (string, error) {
	if n.Kind != yaml.ScalarNode {
		return "", validation(path, n.Line, "must be a scalar")
	}
	return n.Value, nil
}

func intScalar(n *yaml.Node, path string) (int, error) {
	if n.Kind != yaml.ScalarNode {
		return 0, validation(path, n.Line, "must be an integer")
	}
	value, err := strconv.Atoi(n.Value)
	if err != nil {
		return 0, validation(path, n.Line, "must be an integer")
	}
	return value, nil
}

func boolScalar(n *yaml.Node, path string) (bool, error) {
	if n.Kind != yaml.ScalarNode {
		return false, validation(path, n.Line, "must be a boolean")
	}
	switch strings.ToLower(n.Value) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, validation(path, n.Line, "must be a boolean")
	}
}

func durationScalar(n *yaml.Node, path string) (time.Duration, error) {
	if n.Kind != yaml.ScalarNode {
		return 0, validation(path, n.Line, "must be a duration")
	}
	value, err := time.ParseDuration(n.Value)
	if err != nil {
		return 0, validation(path, n.Line, "must be a Go duration such as 10s or 2m")
	}
	return value, nil
}

func validation(path string, line int, message string) error {
	return &ValidationError{Path: path, Line: line, Message: message}
}

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func resolveProjectDir(configPath, projectDir string) string {
	if filepath.IsAbs(projectDir) {
		return filepath.Clean(projectDir)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(configPath), projectDir))
}
