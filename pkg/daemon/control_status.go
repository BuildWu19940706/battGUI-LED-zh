package daemon

import (
	"errors"
	"sync"
	"time"

	"github.com/charlie0129/batt/pkg/calibration"
	"github.com/charlie0129/batt/pkg/config"
	"github.com/charlie0129/batt/pkg/powerinfo"
	"github.com/charlie0129/batt/pkg/smc"
	"github.com/peterneutron/powerkit-go/pkg/powerkit"
)

var errLEDReadback = errors.New("MagSafe readback did not match requested state")

var controlState = struct {
	sync.Mutex
	tracker   powerinfo.ControlTracker
	snapshot  powerinfo.ControlStatus
	suspended bool
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
	snapshot.LED = powerinfo.LEDStatus{InterfacePresent: caps.MagSafeLEDInterface, Available: caps.MagSafeLED, Reason: caps.MagSafeLEDReason, Mode: string(conf.ControlMagSafeLED()), Desired: "system"}
	desired := smc.LEDSystem
	if in.Valid && in.PluggedIn && !in.Suspended && caps.MagSafeLED {
		switch conf.ControlMagSafeLED() {
		case config.ControlMagSafeModeAlwaysOff:
			desired = smc.LEDOff
			snapshot.LED.Desired = "off"
		case config.ControlMagSafeModeEnabled:
			switch snapshot.State {
			case "charging", "discharging", "calibrating":
				desired = smc.LEDOrange
				snapshot.LED.Desired = "orange"
			case "reached", "holding":
				desired = smc.LEDGreen
				snapshot.LED.Desired = "green"
			}
		}
	}
	statusLED.apply(desired, caps.MagSafeLED, smcConn.SetMagSafeLedState, smcConn.GetMagSafeLedState)
	if statusLED.hasFailed() {
		snapshot.LED.Available = false
		snapshot.LED.Reason = "write_failed"
		snapshot.LED.Desired = "system"
	}
	controlState.snapshot = snapshot
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
