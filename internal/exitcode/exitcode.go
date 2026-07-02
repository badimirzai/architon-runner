package exitcode

const (
	OK                 = 0
	AssertionFailed    = 2
	InvalidConfig      = 3
	EnvironmentFailure = 4
	ExecutionFailure   = 5
	SerialFailure      = 6
	Interrupted        = 130
)

func Name(code int) string {
	switch code {
	case OK:
		return "ok"
	case AssertionFailed:
		return "assertion_failed"
	case InvalidConfig:
		return "invalid_configuration"
	case EnvironmentFailure:
		return "environment_or_dependency_failure"
	case ExecutionFailure:
		return "build_or_upload_execution_failure"
	case SerialFailure:
		return "device_or_serial_communication_failure"
	case Interrupted:
		return "interrupted"
	default:
		return "unknown"
	}
}
