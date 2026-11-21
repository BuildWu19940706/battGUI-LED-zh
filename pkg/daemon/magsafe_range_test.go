package daemon

import (
	"net/http"
	"testing"

	"github.com/charlie0129/batt/pkg/smc"
)

// The range editor controls firmware thresholds. MagSafe independently follows
// the charging flag reported by macOS, even at the configured upper limit.
func TestFirmwareMagSafeFollowsChargingWithCustomRange(t *testing.T) {
	f := newFixture(t, firmwareManaged, withFileConfig(), withCharge(79), withLED(smc.LEDOrange))
	f.mustServe(http.MethodPut, "/charge-range", `{"lower":75,"upper":80}`, http.StatusCreated)
	f.mustServe(http.MethodPut, "/magsafe-led", `"enabled"`, http.StatusCreated)

	for _, sample := range []struct {
		charge   int
		charging bool
		want     smc.MagSafeLedState
	}{
		{79, false, smc.LEDGreen},
		{80, true, smc.LEDOrange},
		{80, false, smc.LEDGreen},
	} {
		f.smc.setCharge(t, sample.charge)
		f.battery.setCharging(sample.charging)
		if !f.d.maintainLoopForced() {
			t.Fatal("firmware maintenance failed")
		}
		if got := f.smc.led(t); got != sample.want {
			t.Errorf("charge=%d charging=%t: LED %v, want %v", sample.charge, sample.charging, got, sample.want)
		}
	}
	if limit := f.smc.firmwareLimit(t); !limit.Active || limit.Lower != 75 || limit.Upper != 80 {
		t.Fatalf("firmware charge range changed: %+v", limit)
	}
}
