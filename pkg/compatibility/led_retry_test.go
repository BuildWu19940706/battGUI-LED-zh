package compatibility

import (
	"encoding/json"
	"testing"
)

func TestLEDCommandCapabilityIncludesManualRecovery(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"magSafeLED":true}`, true},
		{`{"magSafeLED":false,"magSafeLEDRetry":true}`, true},
		{`{"magSafeLED":false,"magSafeLEDInterface":true,"magSafeLEDReason":"firmware_unverified"}`, false},
		{`{"magSafeLED":false,"magSafeLEDReason":"write_failed"}`, false},
	} {
		var caps Capabilities
		if err := json.Unmarshal([]byte(tc.body), &caps); err != nil {
			t.Fatal(err)
		}
		if caps.Supports(FeatureMagSafeLED) != tc.want {
			t.Fatalf("incorrect command availability for %s", tc.body)
		}
	}
}
