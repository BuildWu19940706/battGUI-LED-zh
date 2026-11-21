package gui

import (
	"fmt"

	"github.com/charlie0129/batt/pkg/config"
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

// magSafeModeTitle describes the configured upstream mode, not a sampled LED color.
func magSafeModeTitle(available bool, mode config.ControlMagSafeMode) string {
	label := "LED control unavailable"
	if available {
		switch mode {
		case config.ControlMagSafeModeEnabled:
			label = "Enable"
		case config.ControlMagSafeModeAlwaysOff:
			label = "Always-off"
		default:
			label = "Disable"
		}
	}
	return fmt.Sprintf(tr("MagSafe: %s"), tr(label))
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
	conf := config.NewFileFromConfig(raw, "")
	status := tr("State: Unknown")
	if telemetry, err := c.api.GetTelemetry(false, false); err == nil && telemetry != nil && telemetry.Control != nil {
		status = controlStatusTitle(telemetry.Control)
	}
	led := magSafeModeTitle(c.capabilities.MagSafeLED, conf.ControlMagSafeLED())
	upper, lower, ok := showChargeRange(conf.UpperLimit(), conf.LowerLimit(), status, led)
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
