package gui

import (
	"fmt"
	"github.com/charlie0129/batt/pkg/powerinfo"
)

func controlStatusTitle(status *powerinfo.ControlStatus) string {
	if status == nil {
		return tr("State: Unknown")
	}
	switch status.State {
	case "charging":
		return fmt.Sprintf(tr("Charging to %d%% · now %d%%"), status.TargetPercent, status.ChargePercent)
	case "reached":
		return fmt.Sprintf(tr("Reached %d%% · now %d%%"), status.TargetPercent, status.ChargePercent)
	case "holding":
		return fmt.Sprintf(tr("Holding %d%%–%d%% · now %d%%"), status.LowerPercent, status.TargetPercent, status.ChargePercent)
	case "on_battery":
		return fmt.Sprintf(tr("On Battery · %d%%"), status.ChargePercent)
	case "discharging":
		return fmt.Sprintf(tr("Discharging · %d%%"), status.ChargePercent)
	case "calibrating":
		return tr("State: Calibrating")
	case "sleeping":
		return tr("State: Sleeping")
	case "paused":
		return tr("Paused · reason unconfirmed")
	default:
		return tr("State: Unknown")
	}
}

func ledStatusTitle(status powerinfo.LEDStatus) string {
	message := "System controlled"
	switch {
	case status.Reason == "release_failed":
		message = "LED release failed; system control could not be confirmed"
	case status.Reason == "write_failed":
		message = "Control failed; returned to system"
	case status.Reason == "firmware_unverified":
		message = "Firmware mode: physical LED control not yet verified"
	case status.Reason == "no_interface":
		message = "No usable LED interface"
	case status.Available:
		switch status.Mode {
		case "enabled":
			message = "Indicate charge target"
		case "always-off":
			message = "Always off"
		}
	default:
		message = "LED control unavailable"
	}
	return fmt.Sprintf(tr("MagSafe: %s"), tr(message))
}

func validChargeRange(upper, lower int) bool {
	return upper >= 10 && upper <= 100 && lower >= 10 && lower < upper
}

func (c *menuController) editChargeRange() {
	raw, err := c.api.GetConfig()
	if err != nil {
		showAlert("Failed to get config", err.Error())
		return
	}
	upper, lower := c.upperLimit, 0
	if raw.Limit != nil {
		upper = *raw.Limit
	}
	delta := 2
	if raw.LowerLimitDelta != nil {
		delta = *raw.LowerLimitDelta
	}
	lower = upper - delta
	status, led := tr("State: Unknown"), tr("LED control unavailable")
	if telemetry, err := c.api.GetTelemetry(false, false); err == nil && telemetry.Control != nil {
		status = controlStatusTitle(telemetry.Control)
		led = ledStatusTitle(telemetry.Control.LED)
	}
	upper, lower, ok := showChargeRange(upper, lower, status, led)
	if !ok {
		return
	}
	if !validChargeRange(upper, lower) {
		showAlert("Invalid charge range", "Use whole percentages satisfying 10 <= lower < upper <= 100.")
		return
	}
	if response, err := c.api.SetChargeRange(upper, lower); err != nil {
		showAlert("Could not save charge range", response+"\n"+err.Error())
		return
	}
	c.refreshOnOpen()
	c.updateTelemetry()
}
