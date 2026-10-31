# Verified Firmware MagSafe LED Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose opt-in MagSafe control on the exact M4 platform whose administrator write/readback and visible green LED the user confirmed on 2026-11-01.

**Architecture:** Read a small hardware/OS profile once before daemon capability detection. Add the verified firmware profile to LED availability independently of the charging backend. Reuse the existing state tracker, serialized writer, sleep/connection release, readback verification and session-failure fallback.

**Tech Stack:** Go 1.24.5, existing native Cocoa GUI, bounded read-only macOS system_profiler/sw_vers.

**Spec:** `docs/superpowers/specs/2026-11-01-batt-zh-core-design.md`, plus the user's successful UID0 orange4/green3/system0 test and physical observation.

## Global Constraints

- Verified tuple: model `Mac16,12`, firmware `mBoot-20457.1.29`, macOS `15.8.1`. Other tuples stay unverified.
- Keep firmware-managed charging and existing 75%–80% config. Never use adapter mode to obtain LED colors.
- LED mode remains opt-in; preserve configured modes across matching-profile daemon restarts. Firmware/native profiles not verified still fall back.
- Profile discovery is read-only, bounded, never prints serial/UUID data, and is not repeated in the poll loop.
- Basic write/physical confirmation is established; sleep/replug/long-term physical stability remains to be observed. Automated tests use mocked hardware only.

## Review Focus

- Missing or changed profile cannot enable LED control or change charge-control backend.
- Profile lookup failure must not prevent the charging daemon from starting.
- Existing configured LED preference survives restart on the verified profile.
- Default-disabled mode must not enable colored LEDs automatically.
- Sleep and connection handling work for firmware mode even though legacy SleepHooks remains false.

### Task 1: Profile-gated capability

**Files:** Add `pkg/daemon/firmware_led.go` and tests; update daemon startup and capability detection.

**Interfaces:** Private `firmwareLEDProfile`, `parseFirmwareLEDProfile`, `supportsFirmwareLED`, and startup-only `currentFirmwareLEDProfile`; no new config or public API fields.

- [x] Write failing tests for exact-match, mismatches/empty profile, JSON parsing, firmware capability isolation, default mode and persisted preference.
- [x] Read/profile once with a timeout, enable LED capability on the tested tuple only, and preserve all charge capabilities.
- [x] Run affected daemon tests and verify no real hardware writes occur.

### Task 2: GUI and delivery

**Files:** Existing GUI control-state helper/controller and shared Chinese resource; Chinese guide and validation record.

- [x] Add localized “Enable (Firmware Mode)” and explain basic validation versus pending long-term checks; test the visible title.
- [x] Run full tests, race detection, vet and independent review; resolve important findings.
- [x] Build and verify `v0.8.0-zh.2`, preserving the .1 archives for rollback, and package matching source and checksums.
- [x] Provide the exact menu path after the user upgrades the daemon; do not replace the installed app or enable colors during development.
