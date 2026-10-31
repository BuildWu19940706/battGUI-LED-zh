package gui

import (
	"github.com/charlie0129/batt/pkg/compatibility"
	"testing"
)

func TestFirmwareLEDEnableTitle(t *testing.T) {
	if got := magSafeEnableTitle(compatibility.ChargeControlFirmware); got != "Enable (Firmware Mode)" {
		t.Fatal(got)
	}
	if got := magSafeEnableTitle(compatibility.ChargeControlLegacy); got != "Enable" {
		t.Fatal(got)
	}
	old := activeLocale
	activeLocale = "zh-Hans"
	t.Cleanup(func() { activeLocale = old })
	if got := magSafeEnableTitle(compatibility.ChargeControlFirmware); got != "启用（固件模式）" {
		t.Fatal(got)
	}
}
