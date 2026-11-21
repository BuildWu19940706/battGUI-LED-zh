package daemon

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/charlie0129/batt/pkg/powerinfo"
)

func TestControlTelemetryTracksAnUninterruptedChargeSession(t *testing.T) {
	f := newFixture(t, firmwareManaged, withCharge(80), withCharging(false))
	readState := func(want string) {
		t.Helper()
		f.d.loopOnce(time.Now())
		response := f.mustServe(http.MethodGet, "/telemetry?power=0&calibration=0", "", http.StatusOK)
		var telemetry struct {
			Control *powerinfo.ControlStatus `json:"control"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &telemetry); err != nil {
			t.Fatal(err)
		}
		if telemetry.Control == nil || telemetry.Control.State != want {
			t.Fatalf("control = %+v, want %s", telemetry.Control, want)
		}
	}
	readState("reached")
	f.smc.setCharge(t, 79)
	readState("holding")
	daemonSleepHandler{f.d}.PowerConnectionChanged()
	readState("paused")
	f.smc.setCharge(t, 80)
	readState("reached")
	f.smc.setCharge(t, 79)
	f.d.systemWillSleep()
	readState("sleeping")
	f.d.systemHasPoweredOn()
	readState("paused")
	f.battery.setErr(errors.New("no battery data"))
	readState("unknown")
}

func TestControlTelemetryInvalidatesOnTargetChange(t *testing.T) {
	for _, tc := range []struct{ name, method, path, body string }{
		{"range", http.MethodPut, "/charge-range", `{"upper":90,"lower":85}`},
		{"upper", http.MethodPut, "/limit", "90"},
		{"lower", http.MethodPut, "/lower-limit-delta", "5"},
		{"charge once", http.MethodPost, "/charge-once/full", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, firmwareManaged, withCharge(80), withCharging(false))
			f.d.loopOnce(time.Now())
			if got := f.d.getControlStatus(); got.State != "reached" {
				t.Fatalf("initial state = %+v", got)
			}
			f.mustServe(tc.method, tc.path, tc.body, http.StatusCreated)
			if got := f.d.getControlStatus(); got.State != "unknown" {
				t.Fatalf("old control snapshot after mutation: %+v", got)
			}
		})
	}
}

func TestNativeControlTelemetryReportsOnlyConfirmedTargets(t *testing.T) {
	f := newExactNativeFixture(t, withCharge(80), withCharging(false))
	f.d.loopOnce(time.Now())
	if got := f.d.getControlStatus(); got.State != "reached" || got.LowerPercent != 80 {
		t.Fatalf("native macOS reported a lower threshold: %+v", got)
	}
	f.smc.setCharge(t, 79)
	f.d.loopOnce(time.Now())
	if got := f.d.getControlStatus(); got.State != "paused" {
		t.Fatalf("native macOS reported a user-defined holding range: %+v", got)
	}
	f.smc.setCharge(t, 83)
	f.battery.snapshot.Voltage, f.battery.snapshot.Amperage = 12, -1
	f.d.loopOnce(time.Now())
	if got := f.d.getControlStatus(); got.State != "discharging" {
		t.Fatalf("native discharge toward a lower target reported %+v", got)
	}
	f.battery.snapshot.Amperage = 0
	f.mustServe(http.MethodPut, "/no-drain", "true", http.StatusCreated)
	f.asserts.foreign = noDrainCaffeinate()
	f.mustServe(http.MethodPut, "/limit", "70", http.StatusCreated)
	f.d.loopOnce(time.Now())
	if got := f.d.getControlStatus(); got.State != "unknown" || f.native.noDrainLimit != 80 {
		t.Fatalf("deferred target reported %+v while the Mac enforces %d", got, f.native.noDrainLimit)
	}
	f.asserts.foreign = nil
	f.smc.setCharge(t, 70)
	f.d.loopOnce(time.Now())
	if got := f.d.getControlStatus(); got.State != "reached" || got.TargetPercent != 70 || f.native.noDrainLimit != 70 {
		t.Fatalf("applied target reported %+v while the Mac enforces %d", got, f.native.noDrainLimit)
	}
}
