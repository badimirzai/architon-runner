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

### 1. Install the CLI

From this repository, install `architon`:

```sh
go install ./cmd/architon
```

Go installs the binary into `$(go env GOPATH)/bin`. On most Macs that is `~/go/bin`.

If your terminal says `zsh: command not found: architon`, add that directory to your `PATH`:

```sh
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Then verify:

```sh
architon version
```

### 2. Check PlatformIO

Make sure PlatformIO is installed and visible:

```sh
pio --version
```

Then go to your firmware project, the directory that contains `platformio.ini`:

```sh
cd /path/to/your/platformio/project
```

### 3. Create the test file

Generate a starter config:

```sh
architon init
```

`architon init` creates `architon.test.yaml`. If `platformio.ini` contains an environment like `[env:esp32-s3-devkitc-1]`, Architon uses that automatically. Otherwise pass it explicitly:

```sh
architon init --environment esp32-s3-devkitc-1
```

If the file already exists, `init` refuses to overwrite it. To replace it:

```sh
architon init --force
```

The generated file looks like this:

```yaml
version: 1
name: my-firmware-smoke-test
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

Edit the assertion values so they match text your firmware actually prints over serial. For the default file, your firmware must print `System ready`.

### 4. Validate and run

```sh
architon validate
```

```sh
architon test --verbose
```

If more than one plausible serial port is connected, Architon Runner fails safely and prints the candidates. Re-run with an explicit port:

```sh
architon test --port /dev/cu.usbmodem1101
```

On macOS, ESP32 serial ports usually look like:

```sh
ls /dev/cu.usbmodem* /dev/cu.usbserial* 2>/dev/null
```

## Commands

```sh
architon version
architon init
architon init --environment esp32-s3-devkitc-1
architon init --file architon.test.yaml --force
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
