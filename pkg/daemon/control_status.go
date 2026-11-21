package daemon

import (
	"sync"
	"time"

	"github.com/charlie0129/batt/pkg/compatibility"
	"github.com/charlie0129/batt/pkg/powerinfo"
)

// controlStatus holds the GUI's evidence of reaching a target during the
// current uninterrupted power connection. It only observes charging.
type controlStatus struct {
	mu        sync.Mutex
	tracker   powerinfo.ControlTracker
	snapshot  powerinfo.ControlStatus
	suspended bool
}

func (d *Daemon) refreshControlStatus(applied bool) {
	d.transitionMu.Lock()
	defer d.transitionMu.Unlock()
	d.loopMu.Lock()
	defer d.loopMu.Unlock()
	if d.chargeControlStopped {
		return
	}

	calibrating := d.calibrationOwnsChargeLimit()
	caps := d.getCapabilities()
	s := &d.control
	s.mu.Lock()
	defer s.mu.Unlock()

	upper, lower := d.conf.UpperLimit(), d.conf.LowerLimit()
	in := powerinfo.ControlInput{
		Now: time.Now(), ConfiguredUpper: upper, Target: upper, Lower: lower,
		Calibrating: calibrating, Suspended: s.suspended,
	}
	switch caps.ChargeControlMode {
	case compatibility.ChargeControlAdapter:
		if upper < 100 {
			in.Lower = min(lower, upper-adapterMinBand)
		}
	case compatibility.ChargeControlNative:
		// macOS exposes one target. It does not report the lower threshold.
		in.NativeLimit = true
		in.Lower = upper
	}
	if target := d.conf.ChargeOnceTarget(); target > 0 && !calibrating {
		in.Target, in.Lower = target, target
	}
	charge, chargeErr := d.smc.GetBatteryCharge()
	plugged, powerErr := d.smc.IsPluggedIn()
	battery, batteryErr := d.battery.Snapshot()
	in.Valid = applied && caps.ChargingControl && chargeErr == nil && powerErr == nil && batteryErr == nil
	in.Charge, in.PluggedIn = charge, plugged
	in.Charging, in.Full = battery.Charging, battery.FullyCharged
	in.Discharging = battery.BatteryPower() < -0.5
	if caps.AdapterControl {
		enabled, err := d.smc.IsAdapterEnabled()
		in.Valid = in.Valid && err == nil
		in.ForcedDischarge = !enabled
	}
	if caps.ChargeControlMode == compatibility.ChargeControlNative {
		if override, err := d.battery.FullChargeOverride(); err == nil && override {
			in.Target, in.Lower = 100, 100
		}
		if plugged && (d.pendingNativeLimit.Load() != 0 || d.getNoDrainStatus().Phase == noDrainWaiting) {
			in.Valid = false
		}
	}
	s.snapshot = s.tracker.Update(in)
}

func (d *Daemon) getControlStatus() powerinfo.ControlStatus {
	s := &d.control
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.SampledAt.IsZero() || time.Since(s.snapshot.SampledAt) > 30*time.Second {
		s.reset()
	}
	return s.snapshot
}

func (s *controlStatus) reset() {
	s.tracker.Reset()
	s.snapshot = powerinfo.ControlStatus{State: "unknown", Reason: "data_unavailable"}
}

func (d *Daemon) invalidateControlStatus() {
	s := &d.control
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset()
}

func (d *Daemon) suspendControlStatus(suspended bool) {
	s := &d.control
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suspended = suspended
	s.reset()
}
