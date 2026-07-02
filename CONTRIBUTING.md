# Contributing

Architon Runner v0.1.0 is intentionally narrow. Changes should preserve local-only deterministic ESP32 hardware testing through PlatformIO and serial assertions.

Before opening a pull request:

```sh
make fmt
make vet
make test
make race
make build
```

If `golangci-lint` is installed locally, also run:

```sh
make lint
```

## Scope Guidelines

Do not add AI, cloud services, user accounts, databases, Studio integration, GitHub Apps, remote execution, generic workflow automation, Arduino CLI, STM32 support, graphical interfaces, arbitrary shell-command workflows, hardware power switching or parallel device execution to v0.1.0.

Prefer direct Go code and small package boundaries. Introduce interfaces only at real system boundaries that need test doubles, such as process execution, serial access and time.

## Tests

The automated test suite must not require physical hardware. Use fakes from `internal/testutil` for process and serial behavior.

Physical ESP32 testing is manual for this release.
