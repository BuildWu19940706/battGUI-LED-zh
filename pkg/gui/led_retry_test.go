package gui

import (
	"testing"

	"github.com/charlie0129/batt/pkg/compatibility"
)

func TestLEDRetryControlsRemainReachable(t *testing.T) {
	for _, tc := range []struct {
		name               string
		installed, upgrade bool
		caps               compatibility.Capabilities
		want               bool
	}{
		{"healthy", true, false, compatibility.Capabilities{ChargingControl: true, MagSafeLED: true}, true},
		{"released failure", true, false, compatibility.Capabilities{ChargingControl: true, MagSafeLEDRetry: true, MagSafeLEDReason: "write_failed"}, true},
		{"release failure", true, false, compatibility.Capabilities{ChargingControl: true, MagSafeLEDRetry: true, MagSafeLEDReason: "release_failed"}, true},
		{"unverified interface", true, false, compatibility.Capabilities{ChargingControl: true, MagSafeLEDInterface: true, MagSafeLEDReason: "firmware_unverified"}, false},
		{"old failed daemon", true, false, compatibility.Capabilities{ChargingControl: true, MagSafeLEDReason: "write_failed"}, false},
		{"upgrade required", true, true, compatibility.Capabilities{ChargingControl: true, MagSafeLEDRetry: true}, false},
		{"not installed", false, false, compatibility.Capabilities{ChargingControl: true, MagSafeLEDRetry: true}, false},
		{"charging unsupported", true, false, compatibility.Capabilities{MagSafeLEDRetry: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := canShowMagSafeControls(tc.installed, tc.caps, tc.upgrade); got != tc.want {
				t.Fatalf("visible=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestLEDRetryActionIsLocalized(t *testing.T) {
	old := activeLocale
	t.Cleanup(func() { activeLocale = old })
	for _, tc := range []struct{ locale, want string }{{"en", "Retry LED Control"}, {"zh-Hans", "重新接管灯控"}} {
		activeLocale = tc.locale
		if got := magSafeActionTitle(compatibility.Capabilities{MagSafeLEDRetry: true}); got != tc.want {
			t.Fatalf("locale=%s title=%s", tc.locale, got)
		}
	}
}
