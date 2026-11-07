package daemon

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charlie0129/batt/pkg/compatibility"
	"github.com/charlie0129/batt/pkg/config"
	"github.com/charlie0129/batt/pkg/powerinfo"
	"github.com/charlie0129/batt/pkg/smc"
	"github.com/charlie0129/gosmc"
)

func useLEDRecoveryState(t *testing.T, pending bool, data []byte) (*config.File, string) {
	t.Helper()
	led, err := gosmc.NewValue(smc.MagSafeLedKey, gosmc.DataType("hex_"), data)
	if err != nil {
		t.Fatal(err)
	}
	useMockSMC(t, led)
	file, path := useTempConfig(t)
	file.SetChargeRange(80, 75)
	file.SetControlMagSafeLED(config.ControlMagSafeModeEnabled)
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	oldCaps, oldCharger := loadChargeControl()
	t.Cleanup(func() {
		maintainLoopInnerLock.Lock()
		defer maintainLoopInnerLock.Unlock()
		storeChargeControl(oldCaps, oldCharger)
	})
	setChargeControl(compatibility.Capabilities{ChargingControl: true, ChargeControlMode: compatibility.ChargeControlFirmware, ChargeRange: true, MagSafeLED: true, MagSafeLEDInterface: true})
	statusLED.mu.Lock()
	failed, owned, known, releasePending, attempts, last := statusLED.failed, statusLED.owned, statusLED.known, statusLED.releasePending, statusLED.releaseAttempts, statusLED.last
	statusLED.failed, statusLED.known, statusLED.owned, statusLED.releasePending = true, true, pending, pending
	statusLED.releaseAttempts, statusLED.last = 3, smc.LEDSystem
	statusLED.mu.Unlock()
	t.Cleanup(func() {
		statusLED.mu.Lock()
		statusLED.failed, statusLED.owned, statusLED.known, statusLED.releasePending, statusLED.releaseAttempts, statusLED.last = failed, owned, known, releasePending, attempts, last
		statusLED.mu.Unlock()
	})
	seedReachedControl(t)
	controlState.Lock()
	controlState.snapshot.LED = powerinfo.LEDStatus{InterfacePresent: true, Reason: "write_failed", Mode: "enabled", Desired: "system"}
	controlState.Unlock()
	return file, path
}

func requestLEDRoute(method, path, body string) *httptest.ResponseRecorder {
	res := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setupRoutes().ServeHTTP(res, req)
	return res
}

func TestLEDManualEnableRecoversFailedSession(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "already released", true: "release attempts exhausted"}[pending], func(t *testing.T) {
			_, path := useLEDRecoveryState(t, pending, []byte{3})
			res := requestLEDRoute(http.MethodPut, "/magsafe-led", `"enabled"`)
			if res.Code != http.StatusCreated {
				t.Fatalf("status=%d body=%s", res.Code, res.Body)
			}
			if statusLED.hasFailed() || statusLED.hasPendingRelease() {
				t.Fatal("explicit enable left the failed session locked out")
			}
			got, err := smcConn.GetMagSafeLedState()
			if err != nil || got != smc.LEDSystem {
				t.Fatalf("recovery did not establish system baseline: state=%d err=%v", got, err)
			}
			stored, err := config.NewFile(path)
			if err != nil || stored.ControlMagSafeLED() != config.ControlMagSafeModeEnabled || stored.LowerLimit() != 75 || stored.UpperLimit() != 80 {
				t.Fatalf("lost existing settings: %v", err)
			}
			if got := getControlStatus(); got.LED.Reason != "" || !got.LED.Available || got.LED.Desired != "system" || got.State != "reached" {
				t.Fatalf("stale failure or lost target evidence: %+v", got)
			}
			statusLED.apply(smc.LEDGreen, true, smcConn.SetMagSafeLedState, smcConn.GetMagSafeLedState)
			got, err = smcConn.GetMagSafeLedState()
			if err != nil || got != smc.LEDGreen || statusLED.hasFailed() {
				t.Fatalf("new session could not take control: state=%d err=%v", got, err)
			}
		})
	}
}

func TestLEDRetryCapabilityRequiresVerifiedHardware(t *testing.T) {
	useLEDRecoveryState(t, false, []byte{3})
	for _, verified := range []bool{true, false} {
		caps := getCapabilities()
		caps.MagSafeLED = verified
		setChargeControl(caps)
		res := requestLEDRoute(http.MethodGet, "/compatibility", "")
		var got struct {
			Available bool `json:"magSafeLED"`
			Retry     bool `json:"magSafeLEDRetry"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Available || got.Retry != verified {
			t.Fatalf("availability/retry=%+v verified=%v", got, verified)
		}
	}
}

func TestLEDManualRetryCannotBypassUnsupportedHardware(t *testing.T) {
	useLEDRecoveryState(t, false, []byte{3})
	caps := getCapabilities()
	caps.MagSafeLED = false
	caps.MagSafeLEDReason = "firmware_unverified"
	setChargeControl(caps)
	res := requestLEDRoute(http.MethodPut, "/magsafe-led", `"enabled"`)
	got, _ := smcConn.GetMagSafeLedState()
	if res.Code != http.StatusConflict || !statusLED.hasFailed() || got != smc.LEDGreen {
		t.Fatalf("unverified device was recovered: status=%d state=%d", res.Code, got)
	}
}

func TestLEDManualRetryRequiresSuccessfulSystemRelease(t *testing.T) {
	file, path := useLEDRecoveryState(t, true, []byte{0, 0, 0, 3}) // Interface exists; one-byte write fails.
	file.SetControlMagSafeLED(config.ControlMagSafeModeAlwaysOff)
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	res := requestLEDRoute(http.MethodPut, "/magsafe-led", `"enabled"`)
	stored, err := config.NewFile(path)
	if res.Code != http.StatusInternalServerError || !statusLED.hasFailed() || !statusLED.hasPendingRelease() {
		t.Fatalf("unsafe recovery: status=%d body=%s", res.Code, res.Body)
	}
	if err != nil || stored.ControlMagSafeLED() != config.ControlMagSafeModeAlwaysOff || file.ControlMagSafeLED() != config.ControlMagSafeModeAlwaysOff {
		t.Fatalf("failed recovery did not restore preference: %v", err)
	}
}

type ledSaveFailureConfig struct {
	mockConf
	mode config.ControlMagSafeMode
}

func (f *ledSaveFailureConfig) ControlMagSafeLED() config.ControlMagSafeMode        { return f.mode }
func (f *ledSaveFailureConfig) SetControlMagSafeLED(mode config.ControlMagSafeMode) { f.mode = mode }

func TestLEDManualRetryDoesNotRunWhenConfigSaveFails(t *testing.T) {
	useLEDRecoveryState(t, false, []byte{3})
	f := &ledSaveFailureConfig{mockConf: mockConf{upper: 80, lower: 75, saveErr: errors.New("disk full")}, mode: config.ControlMagSafeModeDisabled}
	conf = f
	res := requestLEDRoute(http.MethodPut, "/magsafe-led", `"enabled"`)
	got, _ := smcConn.GetMagSafeLedState()
	if res.Code != http.StatusInternalServerError || !statusLED.hasFailed() || got != smc.LEDGreen || f.mode != config.ControlMagSafeModeDisabled {
		t.Fatalf("save failure changed LED/session/preference: status=%d state=%d mode=%s", res.Code, got, f.mode)
	}
}

func TestLEDManualRetryRejectsInvalidRequests(t *testing.T) {
	for _, body := range []string{`"invalid"`, `null`, `123`, `{}`, `"broken`} {
		t.Run(body, func(t *testing.T) {
			file, _ := useLEDRecoveryState(t, false, []byte{3})
			res := requestLEDRoute(http.MethodPut, "/magsafe-led", body)
			got, _ := smcConn.GetMagSafeLedState()
			if res.Code != http.StatusBadRequest || !statusLED.hasFailed() || got != smc.LEDGreen || file.ControlMagSafeLED() != config.ControlMagSafeModeEnabled {
				t.Fatalf("invalid request changed session: status=%d state=%d mode=%s", res.Code, got, file.ControlMagSafeLED())
			}
		})
	}
}

func TestLEDManualRetryKeepsLegacyBooleanRequests(t *testing.T) {
	for _, body := range []string{`true`, `false`, `"always-off"`, `"disabled"`} {
		t.Run(body, func(t *testing.T) {
			useLEDRecoveryState(t, false, []byte{3})
			res := requestLEDRoute(http.MethodPut, "/magsafe-led", body)
			if res.Code != http.StatusCreated || statusLED.hasFailed() {
				t.Fatalf("valid legacy/mode request did not recover: status=%d", res.Code)
			}
		})
	}
}
