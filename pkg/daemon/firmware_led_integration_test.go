//go:build darwin && integration

package daemon

import "testing"

// Explicit, read-only host check; excluded from the hermetic default suite.
func TestLocalFirmwareLEDProfile(t *testing.T) {
	profile, err := readFirmwareLEDProfile()
	if err != nil {
		t.Fatal(err)
	}
	if profile.Model == "" || profile.Firmware == "" || profile.OSVersion == "" {
		t.Fatal("incomplete host profile")
	}
	t.Logf("model=%s firmware=%s macOS=%s verifiedLED=%v", profile.Model, profile.Firmware, profile.OSVersion, supportsFirmwareLED(profile))
}
