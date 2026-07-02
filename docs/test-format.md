# Test Format

Architon Runner v0.1.0 supports schema version `1`.

Unknown fields, unsupported steps and unsupported assertions are rejected. Validation errors include the YAML path and line when the YAML parser provides line information.

## Example

```yaml
version: 1
name: esp32-bno055-smoke-test
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
  reset_after_upload: false
steps:
  - build
  - upload
  - monitor:
      duration: 10s
assertions:
  - serial_contains:
      value: "System ready"
      within: 10s
  - serial_contains:
      value: "BNO055 detected"
      within: 10s
  - serial_not_contains:
      value: "Guru Meditation Error"
  - serial_not_contains:
      value: "panic"
      case_sensitive: false
```

## Top-Level Fields

| Field | Required | Description |
| --- | --- | --- |
| `version` | yes | Must be `1`. |
| `name` | yes | Human-readable test name. |
| `project` | yes | PlatformIO project settings. |
| `device` | yes | ESP32 serial device settings. |
| `execution` | no | Timeout and reset settings. Defaults are applied when omitted. |
| `steps` | yes | Ordered list of supported steps. |
| `assertions` | yes | Ordered list of supported serial assertions. |

## Project

| Field | Required | Description |
| --- | --- | --- |
| `directory` | yes | PlatformIO project directory. Relative paths are resolved from the test file location. |
| `environment` | yes | PlatformIO environment passed to `pio run -e`. |

## Device

| Field | Required | Description |
| --- | --- | --- |
| `platform` | yes | Must be `esp32`. |
| `port` | yes | Serial port path or `auto`. |
| `baud` | no | Serial baud rate. Defaults to `115200`. |

`port: auto` succeeds only when exactly one plausible USB serial port is found. If several candidates exist, the run fails and asks for `--port` or `device.port`.

## Execution

| Field | Default | Description |
| --- | ---: | --- |
| `build_timeout` | `120s` | Timeout for `pio run -e <environment>`. |
| `upload_timeout` | `60s` | Timeout for `pio run -e <environment> -t upload --upload-port <port>`. |
| `boot_timeout` | `15s` | Time allowed for the serial port to become openable after upload. |
| `test_timeout` | `30s` | Default deadline for `serial_contains` assertions when `within` is omitted. |
| `reset_after_upload` | `false` | When true, toggles DTR/RTS after opening serial and before capture. |

Durations use Go duration syntax such as `500ms`, `10s` or `2m`.

## Steps

Supported steps are exactly:

- `build`
- `upload`
- `monitor`

`monitor` requires a duration:

```yaml
steps:
  - monitor:
      duration: 10s
```

## Assertions

Supported assertions are exactly:

### `serial_contains`

Passes when the configured string appears before its deadline.

```yaml
- serial_contains:
    value: "System ready"
    within: 10s
    case_sensitive: true
```

Fields:

- `value` is required.
- `within` is optional and defaults to `execution.test_timeout`.
- `case_sensitive` is optional and defaults to `true`.

### `serial_not_contains`

Fails if the configured string appears at any point during the monitor period.

```yaml
- serial_not_contains:
    value: "panic"
    case_sensitive: false
```

Fields:

- `value` is required.
- `case_sensitive` is optional and defaults to `true`.

Matching is exact string matching. There is no LLM, fuzzy matching or regular expression support in v0.1.0.
