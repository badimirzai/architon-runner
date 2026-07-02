package exitcode

import "testing"

func TestStableExitCodeNames(t *testing.T) {
	tests := map[int]string{
		OK:                 "ok",
		AssertionFailed:    "assertion_failed",
		InvalidConfig:      "invalid_configuration",
		EnvironmentFailure: "environment_or_dependency_failure",
		ExecutionFailure:   "build_or_upload_execution_failure",
		SerialFailure:      "device_or_serial_communication_failure",
		Interrupted:        "interrupted",
	}
	for code, want := range tests {
		if got := Name(code); got != want {
			t.Fatalf("Name(%d) = %q, want %q", code, got, want)
		}
	}
}
