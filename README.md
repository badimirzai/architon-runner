# Architon Runner

Architon Runner is a deterministic local hardware test runner for embedded firmware.

The v0.1.0 scope is intentionally narrow: build, flash and smoke-test ESP32 firmware through PlatformIO and USB serial.

It currently supports only:

- ESP32 devices
- PlatformIO projects
- USB serial capture
- `serial_contains` and `serial_not_contains` assertions
- local execution on macOS and Linux

Architon Runner does not replace unit tests, does not prove electrical safety, is not intended for safety-critical qualification, and does not run physical ESP32 tests in hosted GitHub Actions.

## Five-Minute Quick Start

Prerequisites:

- Go 1.25 or newer
- PlatformIO CLI installed and available as `pio`
- An ESP32 board connected over USB
- A PlatformIO firmware project with a valid environment name

From this repository:

```sh
go install ./cmd/architon
```

In your PlatformIO firmware project, create `architon.test.yaml`:

```yaml
version: 1
name: esp32-basic-smoke-test
project:
  directory: .
  environment: esp32-s3-devkitc-1
device:
  platform: esp32
  port: auto
  baud: 115200
execution:
  build_timeout: 120s
  upload_timeout: 60s
  boot_timeout: 15s
  test_timeout: 30s
steps:
  - build
  - upload
  - monitor:
      duration: 10s
assertions:
  - serial_contains:
      value: "System ready"
      within: 10s
  - serial_not_contains:
      value: "panic"
      case_sensitive: false
```

Validate the file:

```sh
architon validate
```

Run the hardware smoke test:

```sh
architon test
```

If more than one plausible serial port is connected, Architon Runner fails safely and prints the candidates. Re-run with an explicit port:

```sh
architon test --port /dev/cu.usbmodem1101
```

## Commands

```sh
architon version
architon validate
architon validate --file architon.test.yaml
architon test
architon test --file architon.test.yaml
architon test --port /dev/cu.usbmodem1101
architon test --verbose
architon test --no-color
```

`NO_COLOR` is honored.

## Exit Codes

| Code | Meaning |
| ---: | --- |
| 0 | all steps and assertions passed |
| 2 | one or more deterministic assertions failed |
| 3 | invalid configuration |
| 4 | environment or dependency failure |
| 5 | build or upload execution failure |
| 6 | device or serial communication failure |
| 130 | interrupted |

## Artifacts

Each valid test run creates `.architon/runs/<UTC timestamp>/` containing:

- `config.resolved.yaml`
- `serial.log`
- `build.stdout.log`
- `build.stderr.log`
- `upload.stdout.log`
- `upload.stderr.log`
- `result.json`

Run artifacts are intentionally gitignored.

## Documentation

- [Test format](docs/test-format.md)
- [Architecture](docs/architecture.md)
- [Troubleshooting](docs/troubleshooting.md)

## Development

```sh
make fmt
make vet
make test
make race
make build
```

`make lint` requires `golangci-lint` to be installed locally.
