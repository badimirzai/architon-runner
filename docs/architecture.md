# Architecture

Architon Runner is a local-only Go CLI. It deliberately avoids cloud services, user accounts, databases, Studio integration, remote execution, generic workflow automation and graphical interfaces.

## Package Layout

- `cmd/architon`: CLI parsing, signal handling and process exit.
- `internal/config`: strict YAML parsing, schema versioning, defaults and resolved config output.
- `internal/exitcode`: centralized stable exit codes.
- `internal/platformio`: PlatformIO process boundary using `os/exec` without a shell.
- `internal/serial`: serial port detection, opening and live hardware adapter.
- `internal/assertion`: deterministic serial assertion evaluator.
- `internal/runner`: test orchestration, artifacts, serial capture and outcome mapping.
- `internal/report`: console report and versioned JSON result schema.
- `internal/testutil`: fake process, serial and clock adapters for tests.

## Runtime Flow

1. Load `architon.test.yaml` or the `--file` path.
2. Strictly validate schema version, fields, steps and assertions.
3. Resolve defaults and CLI overrides.
4. Create `.architon/runs/<UTC timestamp>/`.
5. Write `config.resolved.yaml`.
6. Verify that `pio` exists on `PATH`.
7. Run `pio run -e <environment>`.
8. Select the configured serial port or safely auto-detect one.
9. Run `pio run -e <environment> -t upload --upload-port <port>`.
10. Open the serial port, optionally reset through DTR/RTS, and capture serial output.
11. Evaluate exact serial assertions.
12. Write logs and `result.json`.
13. Print a pass/fail report and return a stable exit code.

## Determinism

Assertions are evaluated from serial bytes observed during the monitor window:

- `serial_contains` passes only if its exact string appears before its deadline.
- `serial_not_contains` fails immediately when its exact string appears.
- Case-insensitive matching is explicit per assertion.
- No AI, fuzzy matching or probabilistic scoring is used.

## Boundaries for Tests

The test suite does not require physical hardware. The runtime has injectable boundaries for:

- process execution
- serial port detection and opening
- serial stream reads
- time

The production runtime uses the real PlatformIO binary and the maintained `go.bug.st/serial` library.

## Artifact Policy

Artifacts are written under `.architon/runs/` and are not intended for source control. The runner stores command output, serial logs, resolved configuration and a versioned JSON result. It does not store environment variables or secrets.
