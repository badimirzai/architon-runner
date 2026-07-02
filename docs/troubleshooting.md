# Troubleshooting

## `PlatformIO CLI not found`

Install PlatformIO and make sure `pio` is available on `PATH`:

```sh
pio --version
```

Then retry:

```sh
architon test
```

## Multiple Serial Ports Found

Auto-detection fails safely when more than one plausible USB serial port exists. Run with an explicit port:

```sh
architon test --port /dev/cu.usbmodem1101
```

Or set it in `architon.test.yaml`:

```yaml
device:
  platform: esp32
  port: /dev/cu.usbmodem1101
  baud: 115200
```

## No Serial Port Found

Check that the ESP32 is connected over USB and that the OS sees a serial device.

On macOS:

```sh
ls /dev/cu.usb*
```

On Linux:

```sh
ls /dev/ttyUSB* /dev/ttyACM*
```

Linux users may need dialout permissions depending on the distribution.

## Build or Upload Fails

Run PlatformIO directly from the project directory:

```sh
pio run -e esp32-s3-devkitc-1
pio run -e esp32-s3-devkitc-1 -t upload --upload-port /dev/cu.usbmodem1101
```

Architon Runner stores full output in:

- `.architon/runs/<timestamp>/build.stdout.log`
- `.architon/runs/<timestamp>/build.stderr.log`
- `.architon/runs/<timestamp>/upload.stdout.log`
- `.architon/runs/<timestamp>/upload.stderr.log`

Use `architon test --verbose` to stream PlatformIO output live.

## Expected Serial Text Is Missing

Check:

- firmware prints the expected text at the configured baud rate
- the monitor duration is long enough
- the `within` deadline is long enough
- the assertion case sensitivity matches the firmware output

The full timestamped serial capture is stored in:

```text
.architon/runs/<timestamp>/serial.log
```

## Forbidden Serial Text Is Observed

`serial_not_contains` fails as soon as the configured text appears during the monitor period. The report and `result.json` include evidence from the serial stream.

## Physical Testing in CI

Hosted GitHub Actions in this repository run formatting, vet, unit tests, race tests, static analysis and builds only. They do not run physical ESP32 hardware tests.
