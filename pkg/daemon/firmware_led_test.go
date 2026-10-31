package daemon

import (
	"github.com/charlie0129/batt/pkg/compatibility"
	"github.com/charlie0129/batt/pkg/config"
	"github.com/charlie0129/batt/pkg/smc"
	"github.com/charlie0129/gosmc"
	"testing"
)

func TestFirmwareLEDProfileGate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile firmwareLEDProfile
		want    bool
	}{
		{"tested M4", firmwareLEDProfile{"Mac16,12", "mBoot-20457.1.29", "15.8.1"}, true},
		{"different model", firmwareLEDProfile{"Mac16,13", "mBoot-20457.1.29", "15.8.1"}, false},
		{"updated firmware", firmwareLEDProfile{"Mac16,12", "mBoot-20457.1.30", "15.8.1"}, false},
		{"updated OS", firmwareLEDProfile{"Mac16,12", "mBoot-20457.1.29", "15.8.2"}, false},
		{"missing profile", firmwareLEDProfile{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := supportsFirmwareLED(tc.profile); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestParseFirmwareLEDProfile(t *testing.T) {
	hardware := []byte(`{"SPHardwareDataType":[{"machine_model":"Mac16,12","boot_rom_version":"mBoot-20457.1.29","serial_number":"not-to-be-retained","platform_UUID":"not-to-be-retained"}]}`)
	got, err := parseFirmwareLEDProfile(hardware, "15.8.1\n")
	if err != nil || !supportsFirmwareLED(got) {
		t.Fatalf("profile=%+v error=%v", got, err)
	}
	for _, bad := range []string{`broken`, `{}`, `{"SPHardwareDataType":[]}`, `{"SPHardwareDataType":[{}]}`} {
		if _, err := parseFirmwareLEDProfile([]byte(bad), "15.8.1"); err == nil {
			t.Fatalf("accepted unusable profile: %s", bad)
		}
	}
}

func TestVerifiedFirmwareLEDKeepsChargingBackendAndPreference(t *testing.T) {
	oldProfile, oldCaps := currentFirmwareLEDProfile, capabilities
	t.Cleanup(func() { currentFirmwareLEDProfile, capabilities = oldProfile, oldCaps })
	currentFirmwareLEDProfile = firmwareLEDProfile{"Mac16,12", "mBoot-20457.1.29", "15.8.1"}
	mock := firmwareChargeMock(t)
	led, err := gosmc.NewValue(smc.MagSafeLedKey, gosmc.TypeUInt8, []byte{0})
	if err != nil {
		t.Fatal(err)
	}
	// Build one mock containing firmware range keys and the LED interface.
	activation, _ := mock.Read(smc.FirmwareChargeLimitActivationKey)
	upper, _ := mock.Read(smc.FirmwareChargeLimitUpperKey)
	lower, _ := mock.Read(smc.FirmwareChargeLimitLowerKey)
	useMockSMC(t, activation, upper, lower, led)
	file, path := useTempConfig(t)
	file.SetChargeRange(80, 75)
	if file.ControlMagSafeLED() != config.ControlMagSafeModeDisabled {
		t.Fatal("LED must remain opt-in")
	}
	got := detectCapabilities()
	if !got.MagSafeLED || got.ChargeControlMode != compatibility.ChargeControlFirmware || got.SleepHooks {
		t.Fatalf("LED capability altered charge/sleep backend: %+v", got)
	}
	capabilities = got
	file.SetControlMagSafeLED(config.ControlMagSafeModeEnabled)
	disableUnsupportedConfiguredFeatures()
	stored, err := config.NewFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ControlMagSafeLED() != config.ControlMagSafeModeEnabled || stored.UpperLimit() != 80 || stored.LowerLimit() != 75 {
		t.Fatal("restart lost LED preference or charge range")
	}
	currentFirmwareLEDProfile.OSVersion = "15.8.2"
	if caps := detectCapabilities(); caps.MagSafeLED || caps.MagSafeLEDReason != "firmware_unverified" {
		t.Fatalf("OS update must require revalidation: %+v", caps)
	}
}
