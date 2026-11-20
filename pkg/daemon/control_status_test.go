package daemon

import (
	"github.com/charlie0129/batt/pkg/calibration"
	"github.com/charlie0129/batt/pkg/config"
	"github.com/charlie0129/batt/pkg/powerinfo"
	"github.com/charlie0129/batt/pkg/smc"
	"testing"
	"time"
)

func seedReachedControl(t *testing.T) {
	t.Helper()
	controlState.Lock()
	previousTracker, previousSnapshot, previousSuspended := controlState.tracker, controlState.snapshot, controlState.suspended
	controlState.tracker.Reset()
	controlState.suspended = false
	controlState.snapshot = controlState.tracker.Update(powerinfo.ControlInput{Valid: true, PluggedIn: true, Target: 80, Lower: 78, Charge: 80, Now: time.Now()})
	controlState.Unlock()
	t.Cleanup(func() {
		controlState.Lock()
		controlState.tracker, controlState.snapshot, controlState.suspended = previousTracker, previousSnapshot, previousSuspended
		controlState.Unlock()
	})
}

func TestRangeEditInvalidatesReachedBeforeNextSample(t *testing.T) {
	configured := &mockConf{upper: 80, lower: 78}
	useFirmwareLoopState(t, firmwareChargeMock(t), configured)
	seedReachedControl(t)
	for _, body := range []string{`{"upper":90,"lower":85}`, `{"upper":80,"lower":78}`} {
		if res := requestChargeRange(body); res.Code != 201 {
			t.Fatalf("edit failed: %d %s", res.Code, res.Body)
		}
		if got := getControlStatus(); got.State == "reached" || got.State == "holding" {
			t.Fatalf("old green returned after target edit: %+v", got)
		}
	}
}

func TestOneTimeTargetInvalidatesReachedBeforeNextSample(t *testing.T) {
	configured := &mockConf{upper: 80, lower: 78}
	useChargeOnceDaemonState(t, configured, calibration.PhaseIdle)
	seedReachedControl(t)
	if err := startChargeOnce(100, 80); err != nil {
		t.Fatal(err)
	}
	if got := getControlStatus(); got.State == "reached" {
		t.Fatalf("one-time target reused old green: %+v", got)
	}
}

func TestPowerConnectionEventInvalidatesBetweenSamples(t *testing.T) {
	seedReachedControl(t)
	observePowerConnection(true)
	observePowerConnection(false)
	observePowerConnection(true)
	controlState.Lock()
	got := controlState.tracker.Update(powerinfo.ControlInput{Valid: true, PluggedIn: true, Target: 80, Lower: 78, Charge: 79, Now: time.Now()})
	controlState.Unlock()
	if got.State != "paused" {
		t.Fatalf("reconnect kept previous reached latch: %+v", got)
	}
}

func TestMagSafeColorAcrossReconnectAndChargeLimit(t *testing.T) {
	var tracker powerinfo.ControlTracker
	for _, sample := range []struct {
		name              string
		charge, target    int
		plugged, charging bool
		want              smc.MagSafeLedState
		label             string
	}{
		{"at limit", 80, 80, true, false, smc.LEDGreen, "green"},
		{"unplugged", 79, 80, false, false, smc.LEDSystem, "system"},
		{"reconnected without charging", 79, 80, true, false, smc.LEDGreen, "green"},
		{"charging below limit", 79, 80, true, true, smc.LEDOrange, "orange"},
		{"charging flag still set at limit", 80, 80, true, true, smc.LEDGreen, "green"},
		{"charging flag still set above limit", 81, 80, true, true, smc.LEDGreen, "green"},
		{"charging once toward full", 81, 100, true, true, smc.LEDOrange, "orange"},
	} {
		in := powerinfo.ControlInput{Valid: true, Now: time.Now(), Charge: sample.charge, Target: sample.target, ConfiguredUpper: 80, Lower: 75, PluggedIn: sample.plugged, Charging: sample.charging}
		state := tracker.Update(in)
		got, label := desiredMagSafeLED(in, state.State, config.ControlMagSafeModeEnabled, true)
		if got != sample.want || label != sample.label {
			t.Errorf("%s: got %v (%s), want %v (%s)", sample.name, got, label, sample.want, sample.label)
		}
	}
}
