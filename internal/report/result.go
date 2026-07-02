package report

import (
	"encoding/json"
	"os"
	"time"
)

const SchemaVersion = 1

type RunResult struct {
	SchemaVersion      int               `json:"schema_version"`
	RunnerVersion      string            `json:"runner_version"`
	TestName           string            `json:"test_name"`
	StartedAt          time.Time         `json:"started_at"`
	CompletedAt        time.Time         `json:"completed_at"`
	TotalDuration      string            `json:"total_duration"`
	ProjectEnvironment string            `json:"project_environment"`
	SerialPort         string            `json:"serial_port,omitempty"`
	Steps              []StepResult      `json:"step_results"`
	Assertions         []AssertionResult `json:"assertion_results"`
	FinalStatus        string            `json:"final_status"`
	ExitCode           int               `json:"exit_code"`
	ArtifactPaths      map[string]string `json:"artifact_paths"`
}

type StepResult struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Duration    string    `json:"duration"`
	Command     []string  `json:"command,omitempty"`
	ExitStatus  *int      `json:"exit_status,omitempty"`
	StdoutPath  string    `json:"stdout_path,omitempty"`
	StderrPath  string    `json:"stderr_path,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type AssertionResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Value    string `json:"value"`
	Duration string `json:"duration"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

func WriteJSON(path string, result RunResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
