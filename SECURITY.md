# Security Policy

## Supported Versions

Architon Runner is pre-1.0. Security fixes are accepted for the current development branch.

## Reporting a Vulnerability

Please report security issues privately to the repository owner rather than opening a public issue.

Include:

- affected version or commit
- operating system
- reproduction steps
- impact
- whether hardware access is required

## Local-Only Boundary

Architon Runner does not intentionally send project data, firmware, serial logs, environment variables or artifacts to remote services.

Run artifacts may contain firmware log output from attached devices. Treat `.architon/runs/` as local diagnostic data and review it before sharing.
