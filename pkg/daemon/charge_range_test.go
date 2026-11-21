package daemon

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/charlie0129/batt/pkg/calibration"
	"github.com/charlie0129/batt/pkg/compatibility"
)

func TestChargeRangeValidation(t *testing.T) {
	const valid = `{"upper":80,"lower":78}`
	for _, tc := range []struct {
		name, body  string
		mode        compatibility.ChargeControlMode
		calibrating bool
		status      int
	}{
		{"calibration", valid, compatibility.ChargeControlFirmware, true, 400},
		{"zero width", `{"upper":80,"lower":80}`, compatibility.ChargeControlFirmware, false, 400},
		{"upper too high", `{"upper":101,"lower":78}`, compatibility.ChargeControlFirmware, false, 400},
		{"missing lower", `{"upper":80}`, compatibility.ChargeControlFirmware, false, 400},
		{"null", `null`, compatibility.ChargeControlFirmware, false, 400},
		{"lower too low", `{"upper":80,"lower":9}`, compatibility.ChargeControlFirmware, false, 400},
		{"fraction", `{"upper":80.5,"lower":78}`, compatibility.ChargeControlFirmware, false, 400},
		{"short adapter band", valid, compatibility.ChargeControlAdapter, false, 400},
		{"unsupported", valid, compatibility.ChargeControlUnsupported, false, 409},
		{"native", valid, compatibility.ChargeControlNative, false, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, firmwareManaged, withCapabilities(compatibility.Capabilities{
				ChargingControl: tc.mode != compatibility.ChargeControlUnsupported, ChargeControlMode: tc.mode,
			}))
			if tc.calibrating {
				f.setCalibration(&calibration.State{Phase: calibration.PhaseCharge})
			}
			before := f.mem.snapshot()
			f.mustServe(http.MethodPut, "/charge-range", tc.body, tc.status)
			if got := f.mem.snapshot(); got != before {
				t.Fatalf("rejected range changed config: %+v", got)
			}
		})
	}
}

func TestChargeRangePersistsAndAppliesTogether(t *testing.T) {
	f := newFixture(t, firmwareManaged, withFileConfig())
	f.file.SetChargeOnceTarget(100)
	f.file.SetDisableTimer(time.Now().Add(time.Hour), 85)
	f.mustServe(http.MethodPut, "/charge-range", `{"upper":70,"lower":60}`, http.StatusCreated)
	saved := f.reloadFile()
	if saved.UpperLimit() != 70 || saved.LowerLimit() != 60 || saved.ChargeOnceTarget() != 0 || !saved.DisableUntil().IsZero() {
		t.Fatalf("incorrect saved range or workflow: %v", saved.LogValue())
	}
	got, err := f.smc.GetFirmwareChargeLimit()
	if err != nil || !got.Active || got.Upper != 70 || got.Lower != 60 {
		t.Fatalf("hardware range = %+v, %v", got, err)
	}
}

func TestChargeRangeApplyFailureRollsBack(t *testing.T) {
	f := newFixture(t, firmwareManaged, withFileConfig())
	until := time.Now().Add(time.Hour)
	f.file.SetChargeRange(80, 78)
	f.file.SetDisableTimer(until, 85)
	f.file.SetChargeOnceTarget(100)
	if err := f.file.Save(); err != nil {
		t.Fatal(err)
	}
	f.smc.fail("EnsureFirmwareChargeLimit", errors.New("cannot apply range"))
	f.mustServe(http.MethodPut, "/charge-range", `{"upper":70,"lower":60}`, http.StatusInternalServerError)
	saved := f.reloadFile()
	if saved.UpperLimit() != 80 || saved.LowerLimit() != 78 || saved.ChargeOnceTarget() != 100 || !saved.DisableUntil().Equal(until) || saved.PreDisableLimit() != 85 {
		t.Fatalf("rollback lost config: %v", saved.LogValue())
	}
	if f.smc.callCount("EnsureFirmwareChargeLimitDisabled") != 1 {
		t.Fatal("rollback did not restore one-time full charging on hardware")
	}
}

func TestChargeRangeSaveFailurePreservesWorkflow(t *testing.T) {
	f := newFixture(t, firmwareManaged)
	f.mem.SetDisableTimer(time.Now().Add(time.Hour), 80)
	f.mem.SetChargeOnceTarget(100)
	before := f.mem.snapshot()
	f.mem.failSave(errors.New("disk full"))
	f.mustServe(http.MethodPut, "/charge-range", `{"upper":70,"lower":65}`, http.StatusInternalServerError)
	if got := f.mem.snapshot(); got != before {
		t.Fatalf("failed transaction lost config: %+v", got)
	}
	if f.smc.callCount("EnsureFirmwareChargeLimit") != 0 {
		t.Fatal("failed save applied the range")
	}
}
