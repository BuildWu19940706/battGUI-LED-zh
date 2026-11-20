package daemon

import (
	"errors"
	"sync"
	"time"

	"github.com/charlie0129/batt/pkg/calibration"
	"github.com/charlie0129/batt/pkg/compatibility"
	"github.com/charlie0129/batt/pkg/config"
	"github.com/charlie0129/batt/pkg/powerinfo"
	"github.com/charlie0129/batt/pkg/smc"
	"github.com/peterneutron/powerkit-go/pkg/powerkit"
)

var errLEDReadback = errors.New("MagSafe readback did not match requested state")

var controlState = struct {
	sync.Mutex
	tracker         powerinfo.ControlTracker
	snapshot        powerinfo.ControlStatus
	suspended       bool
	connectionKnown bool
	connected       bool
}{}
var statusLED ledController

// refreshControlStatus is run after the charging loop, never from automated
// loop tests or the GUI. Charging decisions and LED decisions stay independent.
func refreshControlStatus() {
	chargeControlTransitionMu.Lock()
	defer chargeControlTransitionMu.Unlock()
	maintainLoopInnerLock.Lock()
	defer maintainLoopInnerLock.Unlock()
	if smcConn == nil || conf == nil {
		return
	}
	in := powerinfo.ControlInput{Now: time.Now(), Target: conf.UpperLimit(), ConfiguredUpper: conf.UpperLimit(), Lower: conf.LowerLimit()}
	if target := conf.ChargeOnceTarget(); target > 0 {
		in.Target = target
		in.Lower = target
	}
	charge, e1 := smcConn.GetBatteryCharge()
	plugged, e2 := smcConn.IsPluggedIn()
	in.Charge, in.PluggedIn = charge, plugged
	info, e3 := powerkit.GetSystemInfo(powerkit.FetchOptions{QueryIOKit: true, QuerySMC: false})
	in.Valid = e1 == nil && e2 == nil && e3 == nil && info != nil && info.IOKit != nil
	if in.Valid {
		in.Charging = info.IOKit.State.IsCharging
		in.Full = info.IOKit.State.FullyCharged
		in.Discharging = info.IOKit.Calculations.BatteryPower < -0.5
	}
	caps := getCapabilities()
	if caps.AdapterControl {
		enabled, err := smcIsAdapterEnabled()
		if err != nil {
			in.Valid = false
		} else {
			in.ForcedDischarge = !enabled
		}
	}
	calibrationMu.Lock()
	in.Calibrating = calibrationState.Phase != calibration.PhaseIdle
	calibrationMu.Unlock()
	controlState.Lock()
	defer controlState.Unlock()
	in.Suspended = controlState.suspended
	snapshot := controlState.tracker.Update(in)
	mode := conf.ControlMagSafeLED()
	desired, name := desiredMagSafeLED(in, snapshot.State, mode, caps.MagSafeLED)
	statusLED.apply(desired, caps.MagSafeLED, smcConn.SetMagSafeLedState, smcConn.GetMagSafeLedState)
	snapshot.LED = currentLEDStatus(caps, mode, name)
	controlState.snapshot = snapshot
}

func desiredMagSafeLED(in powerinfo.ControlInput, state string, mode config.ControlMagSafeMode, available bool) (smc.MagSafeLedState, string) {
	if !available || !in.Valid || !in.PluggedIn || in.Suspended {
		return smc.LEDSystem, "system"
	}
	switch mode {
	case config.ControlMagSafeModeAlwaysOff:
		return smc.LEDOff, "off"
	case config.ControlMagSafeModeEnabled:
		if state == "unknown" {
			return smc.LEDSystem, "system"
		}
		// 每次采样按当前充电标志和有效目标选色，重新插电无需等待新的达标记录。
		if in.Charging && in.Charge < in.Target {
			return smc.LEDOrange, "orange"
		}
		return smc.LEDGreen, "green"
	default:
		return smc.LEDSystem, "system"
	}
}

func currentLEDStatus(caps compatibility.Capabilities, mode config.ControlMagSafeMode, desired string) powerinfo.LEDStatus {
	status := powerinfo.LEDStatus{InterfacePresent: caps.MagSafeLEDInterface, Available: caps.MagSafeLED, Reason: caps.MagSafeLEDReason, Mode: string(mode), Desired: desired}
	if failed, pending := statusLED.failureState(); failed {
		status.Available, status.Desired, status.Reason = false, "system", "write_failed"
		if pending {
			status.Reason = "release_failed"
		}
	}
	return status
}

// Refresh LED telemetry after a manual baseline attempt without resetting the
// independent evidence that the charge target has been reached.
func recordLEDRecovery() {
	controlState.Lock()
	defer controlState.Unlock()
	controlState.snapshot.LED = currentLEDStatus(getCapabilities(), conf.ControlMagSafeLED(), "system")
}

func getControlStatus() powerinfo.ControlStatus {
	controlState.Lock()
	defer controlState.Unlock()
	snapshot := controlState.snapshot
	if snapshot.SampledAt.IsZero() || time.Since(snapshot.SampledAt) > 30*time.Second {
		snapshot.State, snapshot.Reason = "unknown", "data_unavailable"
		controlState.tracker.Reset()
	}
	return snapshot
}

func suspendControlStatus(suspended bool) {
	controlState.Lock()
	defer controlState.Unlock()
	controlState.suspended = suspended
	controlState.tracker.Reset()
	controlState.snapshot.State, controlState.snapshot.Reason = "unknown", "data_unavailable"
	controlState.snapshot.SampledAt = time.Time{}
	if smcConn != nil {
		statusLED.apply(smc.LEDSystem, false, smcConn.SetMagSafeLedState, smcConn.GetMagSafeLedState)
	}
}

// Invalidate synchronously at the mutation boundary, including changes away
// and back between polling samples. Do not change the actual sleep state.
func invalidateControlStatus() {
	controlState.Lock()
	defer controlState.Unlock()
	controlState.tracker.Reset()
	controlState.snapshot.State, controlState.snapshot.Reason = "unknown", "data_unavailable"
	controlState.snapshot.SampledAt = time.Time{}
	if smcConn != nil {
		statusLED.apply(smc.LEDSystem, false, smcConn.SetMagSafeLedState, smcConn.GetMagSafeLedState)
	}
}

func observePowerConnection(connected bool) {
	controlState.Lock()
	changed := !controlState.connectionKnown || controlState.connected != connected
	controlState.connectionKnown, controlState.connected = true, connected
	controlState.Unlock()
	if changed {
		invalidateControlStatus()
	}
}
