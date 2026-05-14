---
phase: 04-recovery-observability-deployment
plan: 03
subsystem: ops
tags: [docs, docker, systemd, deployment]
requires:
  - phase: 04-recovery-observability-deployment
    provides: runtime behavior and retry/recovery semantics that deployment docs must describe
provides:
  - Repo-level operator README for host-binary and Docker deployment
  - Dockerfile for the real CLI entrypoint
  - systemd unit for Linux host deployment
  - Example config using env-backed AdGuard credentials
affects: [phase-04-deployment, operator-docs]
tech-stack:
  added: []
  patterns: [real CLI contract in artifacts, explicit socket and state mounts]
key-files:
  created:
    - README.md
    - Dockerfile
    - deploy/systemd/docker-dns-sync.service
    - testdata/config/example.toml
  modified: []
key-decisions:
  - "Document the exact `docker-dns-sync -config ...` contract in both host and container deployment paths."
  - "Keep secrets out of example config by using `password_ref` and environment-backed deployment examples."
patterns-established:
  - "Deployment artifacts explicitly call out Docker socket privilege and persistent writable state requirements."
requirements-completed: [OPS-02]
duration: 0 min
completed: 2026-05-13
---

# Phase 4 Plan 3: Deployment Artifacts Summary

**Added first-class host-binary and Docker deployment artifacts so operators can run the daemon without reverse-engineering config, state, or socket requirements.**

## Accomplishments
- Added a repo README with concise configuration guidance and separate host-binary and Docker deployment instructions.
- Added a multi-stage Dockerfile that builds the existing CLI and runs it with `-config`.
- Added a `systemd` unit with explicit restart behavior and state directory expectations.
- Added an example TOML config that matches the current schema and uses `password_ref` instead of embedded secrets.

## Files Created/Modified
- `README.md` - operator setup and deployment instructions.
- `Dockerfile` - image build and runtime entrypoint.
- `deploy/systemd/docker-dns-sync.service` - host-binary service template.
- `testdata/config/example.toml` - schema-matching example config.

## Deviations from Plan

None.

## Issues Encountered

None.

## Self-Check: PASSED
