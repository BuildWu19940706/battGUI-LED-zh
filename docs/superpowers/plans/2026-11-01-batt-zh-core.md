# batt 中文核心增强版 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a bilingual batt.app with atomic charge-range settings and reliable, independently gated status/LED control.

**Architecture:** Extend the existing Go daemon and native Cocoa GUI. The daemon owns range transactions and a shared status/LED controller. Localization resources are shared by Go and Objective-C; existing API/config fields remain compatible.

**Tech Stack:** Go 1.24.5, Objective-C/Cocoa, cgo, Gin, native SMC and powerkit telemetry.

**Spec:** `docs/superpowers/specs/2026-11-01-batt-zh-core-design.md`

## Global Constraints

- Base: `2b9f8b579670b3d07365772f6f8e56b4f53912be`; preserve firmware backend and current 78%–80% settings.
- Simplified Chinese preferred language selects zh-Hans; all others select English. Locale changes require restart.
- Existing CLI/config/API behavior stays compatible. New fields are optional for old peers.
- Hardware-writing tests are never part of the automated suite. Physical LED control requires separate validation; failure falls back to explicit GUI status.
- No new temperature, scheduling, or automatic-discharge feature.

## Review Focus

- Range save/apply failures must restore both values and avoid losing a pending one-time charge or timer.
- A noncharging battery below target must never be called full merely because firmware owns charging.
- Sleep, reconnection, changed targets, stale data and restart must not reuse an old green-light latch.
- An old daemon must not accidentally expose unsafe LED controls or crash the new GUI.
- Both locales must render dynamic percentages, durations, power labels and error dialogs without missing translations or invalid format arguments.

### Task 1: Atomic charge-range transaction

**Files:** Create `pkg/daemon/charge_range.go`, `pkg/daemon/charge_range_test.go`; modify daemon routing and `pkg/client/apis.go`.

**Interfaces:** Produces `PUT /charge-range` with `{upper:int, lower:int}` and client `SetChargeRange(upper, lower int) (string, error)`. Reuse backend capability validation and task conflict rules.

- [ ] Write handler tests for valid persistence, malformed/out-of-range inputs, calibration conflict and rollback on save/apply failures; watch them fail.
- [ ] Implement a serialized transaction preserving timer/charge-once state on failure; test with actual config serialization and mock SMC.
- [ ] Run `go test ./pkg/daemon ./pkg/client`; commit the passing change.

### Task 2: Shared status and independent LED controller

**Files:** Create focused status/controller files and tests in `pkg/daemon` and `pkg/powerinfo`; integrate telemetry, compatibility, loop and sleep hooks.

**Interfaces:** Add optional `control` telemetry containing effective target, state/reason, LED availability/reason. Keep `magSafeLED` as usable automatic control; report LED-interface existence separately.

- [ ] Write transition tests for 78/80 boundaries, charging, target reached, held range, one-time target, unknown pause, discharge, unplug/sleep/restart and invalid telemetry; watch failure.
- [ ] Implement one daemon-owned state machine and serialized LED writer with write deduplication and fail-to-system behavior.
- [ ] Add tests for write/read failure and absence of proven LED support; ensure charge enforcement remains unaffected.
- [ ] Run `go test ./pkg/daemon ./pkg/client ./pkg/compatibility ./pkg/powerinfo`; commit.

### Task 3: Bilingual native GUI and range window

**Files:** Modify `pkg/gui` native/Go code; add shared locale resources; update app packaging.

**Interfaces:** Native range window calls `SetChargeRange`; menu/status consumes optional `control`, falls back safely for old daemons. Chinese/English localization available to both languages.

- [ ] Write language-selection, resource-format and GUI-helper tests first; observe failures.
- [ ] Implement shared localization for existing static/dynamic GUI content, native range editor with backend validation, and clear status/LED-unavailable explanation.
- [ ] Preserve old-peer behavior, native power-value formatting and existing advanced controls; exercise locale resources and helpers.
- [ ] Run `go test ./pkg/gui ./pkg/client` and build `make app`; commit.

### Task 4: Verify, inspect hardware, and package

- [ ] Run `go test ./...`, race tests for affected Go packages, `go vet ./...`, and `make app` with the pinned toolchain.
- [ ] Review the whole branch using a fresh reviewer; fix important issues with regression tests.
- [ ] Verify app locales and native range window visually. Inspect LED key availability and only use a reversible, separately invoked real-device test when appropriate; restore system LED control afterward.
- [ ] If stability cannot be demonstrated, leave physical LED unavailable with explicit reason and deliver the authorized menu-status fallback.
- [ ] Package app and corresponding source with GPL license and Chinese build/install/rollback instructions; record exact verification and hardware limits.
