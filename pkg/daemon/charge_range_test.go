package daemon

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charlie0129/batt/pkg/calibration"
	"github.com/charlie0129/batt/pkg/compatibility"
	"github.com/charlie0129/batt/pkg/config"
)

func TestChargeRangeValidation(t *testing.T) {
	oldConf, oldCaps, oldState := conf, capabilities, calibrationState
	t.Cleanup(func() { conf, capabilities, calibrationState = oldConf, oldCaps, oldState })
	cases := []struct {
		body  string
		caps  compatibility.Capabilities
		phase calibration.Phase
		want  int
	}{
		{`{"upper":80,"lower":78}`, compatibility.Capabilities{ChargingControl: true, ChargeControlMode: compatibility.ChargeControlFirmware}, calibration.PhaseCharge, 400},
		{`{"upper":80,"lower":80}`, compatibility.Capabilities{ChargingControl: true}, calibration.PhaseIdle, 400},
		{`{"upper":101,"lower":78}`, compatibility.Capabilities{ChargingControl: true}, calibration.PhaseIdle, 400},
		{`{"upper":80}`, compatibility.Capabilities{ChargingControl: true}, calibration.PhaseIdle, 400},
		{`null`, compatibility.Capabilities{ChargingControl: true}, calibration.PhaseIdle, 400},
		{`{"upper":80,"lower":9}`, compatibility.Capabilities{ChargingControl: true}, calibration.PhaseIdle, 400},
		{`{"upper":80.5,"lower":78}`, compatibility.Capabilities{ChargingControl: true}, calibration.PhaseIdle, 400},
		{`{"upper":80,"lower":78}`, compatibility.Capabilities{}, calibration.PhaseIdle, 409},
		{`{"upper":80,"lower":78}`, compatibility.Capabilities{ChargingControl: true, ChargeControlMode: compatibility.ChargeControlNative}, calibration.PhaseIdle, 409},
	}
	for _, tc := range cases {
		t.Run(tc.body+string(tc.phase)+string(tc.caps.ChargeControlMode), func(t *testing.T) {
			configured := &mockConf{upper: 80, lower: 78}
			conf = configured
			capabilities = tc.caps
			calibrationState = &calibration.State{Phase: tc.phase}
			res := requestChargeRange(tc.body)
			if res.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", res.Code, tc.want, res.Body)
			}
			if configured.upper != 80 || configured.lower != 78 {
				t.Fatalf("rejected request changed range: %+v", configured)
			}
		})
	}
}

func TestChargeRangePersistsAndAppliesTogether(t *testing.T) {
	configured := &mockConf{upper: 80, lower: 78, chargeOnceTarget: 100}
	mock := firmwareChargeMock(t)
	useFirmwareLoopState(t, mock, configured)
	file, path := useTempConfig(t)
	file.SetChargeOnceTarget(100)
	res := requestChargeRange(`{"upper":70,"lower":60}`)
	if res.Code != 201 {
		t.Fatalf("status=%d body=%s", res.Code, res.Body)
	}
	stored, err := config.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UpperLimit() != 70 || stored.LowerLimit() != 60 || stored.ChargeOnceTarget() != 0 {
		t.Fatalf("incorrect saved range/workflow: %+v", stored.LogrusFields())
	}
	got, err := mock.GetFirmwareChargeLimit()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Active || got.Upper != 70 || got.Lower != 60 {
		t.Fatalf("hardware range=%+v", got)
	}
}

func TestChargeRangeApplyFailureRollsBack(t *testing.T) {
	configured := &mockConf{upper: 80, lower: 78, chargeOnceTarget: 100}
	// No firmware keys: a simulated hardware error after persistence.
	mock := useMockSMC(t)
	useFirmwareLoopState(t, mock, configured)
	file, path := useTempConfig(t)
	file.SetChargeOnceTarget(100)
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	res := requestChargeRange(`{"upper":70,"lower":60}`)
	if res.Code != 500 {
		t.Fatalf("status=%d body=%s", res.Code, res.Body)
	}
	stored, err := config.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UpperLimit() != 80 || stored.LowerLimit() != 78 || stored.ChargeOnceTarget() != 100 {
		t.Fatalf("rollback lost config: %v target=%d", stored.LogrusFields(), stored.ChargeOnceTarget())
	}
}

func requestChargeRange(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/charge-range", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	setupRoutes().ServeHTTP(res, req)
	return res
}

func TestChargeRangeSaveFailurePreservesWorkflow(t *testing.T) {
	oldConf, oldCaps, oldState := conf, capabilities, calibrationState
	t.Cleanup(func() { conf, capabilities, calibrationState = oldConf, oldCaps, oldState })
	until := time.Now().Add(time.Hour)
	configured := &mockConf{upper: 100, lower: 98, disableUntil: until, preDisableLimit: 80, chargeOnceTarget: 100, saveErr: errors.New("disk full")}
	conf = configured
	capabilities = compatibility.Capabilities{ChargingControl: true, ChargeControlMode: compatibility.ChargeControlFirmware}
	calibrationState = &calibration.State{Phase: calibration.PhaseIdle}
	res := requestChargeRange(`{"upper":70,"lower":65}`)
	if res.Code != 500 {
		t.Fatalf("status=%d body=%s", res.Code, res.Body)
	}
	if configured.upper != 100 || configured.lower != 98 || configured.chargeOnceTarget != 100 || !configured.disableUntil.Equal(until) || configured.preDisableLimit != 80 {
		t.Fatalf("failed transaction lost previous configuration: %+v", configured)
	}
}
