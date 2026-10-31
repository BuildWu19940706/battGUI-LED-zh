package gui

import (
	"fmt"
	"os"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/charlie0129/batt/pkg/client"
	"github.com/charlie0129/batt/pkg/config"
)

func (c *menuController) handleAction(item menuItem, checked bool) {
	switch item {
	case itemChargeRange:
		c.editChargeRange()
	case itemInstall, itemUpgrade:
		c.installOrUpgrade()
	case itemLimit50, itemLimit60, itemLimit70, itemLimit80, itemLimit90:
		c.setLimit(quickLimitForItem(item))
	case itemChargeOnceLimit:
		c.callChargeOnce(c.api.ChargeOnceToLimit, tr("Failed to start charging to the limit"))
	case itemChargeOnceFull:
		c.callChargeOnce(c.api.ChargeOnceToFull, tr("Failed to start charging to 100%"))
	case itemChargeOnceCancel:
		c.callChargeOnce(c.api.CancelChargeOnce, tr("Failed to cancel the one-time charge"))
	case itemMagSafeEnabled:
		c.setMagSafeMode(config.ControlMagSafeModeEnabled)
	case itemMagSafeDisabled:
		c.setMagSafeMode(config.ControlMagSafeModeDisabled)
	case itemMagSafeAlwaysOff:
		c.setMagSafeMode(config.ControlMagSafeModeAlwaysOff)
	case itemPreventIdleSleep:
		c.setPreventIdleSleep(checked)
	case itemDisableChargingPreSleep:
		c.setDisableChargingPreSleep(checked)
	case itemPreventSystemSleep:
		c.setPreventSystemSleep(checked)
	case itemPreventSleepOnAdapterDisable:
		c.setPreventSleepOnAdapterDisable(checked)
	case itemForceDischargeStop:
		c.stopForceDischarge()
	case itemForceDischargeIndefinitely:
		c.startForceDischarge(0)
	case itemForceDischarge1Hour,
		itemForceDischarge2Hours,
		itemForceDischarge4Hours,
		itemForceDischarge8Hours:
		c.startForceDischarge(temporaryDisableDuration(item))
	case itemCalibrationStart:
		c.startCalibration()
	case itemCalibrationPause:
		c.pauseCalibration()
	case itemCalibrationResume:
		c.resumeCalibration()
	case itemCalibrationCancel:
		c.cancelCalibration()
	case itemUninstall:
		c.uninstall()
	case itemDisableLimitIndefinitely:
		c.disableLimit()
	case itemDisableLimit1Hour,
		itemDisableLimit2Hours,
		itemDisableLimit4Hours,
		itemDisableLimit8Hours,
		itemDisableLimit12Hours,
		itemDisableLimit24Hours,
		itemDisableLimit2Days,
		itemDisableLimit3Days,
		itemDisableLimit7Days:
		c.disableLimitFor(temporaryDisableDuration(item))
	case itemQuit:
		c.quit()
	}
}

func (c *menuController) installOrUpgrade() {
	executable, err := os.Executable()
	if err != nil {
		logrus.WithError(err).Error(tr("Failed to get executable path"))
		showAlert(tr("Failed to get executable path"), err.Error())
		return
	}
	if err := installDaemon(executable); err != nil {
		logrus.WithError(err).Error(tr("Failed to install daemon"))
		showAlert(tr("Installation failed"), err.Error())
		return
	}
	if err := startAppAtBoot(); err != nil {
		logrus.WithError(err).Error(tr("Failed to start app at boot"))
		showAlert(tr("Failed to start app at boot"), err.Error())
		return
	}
	c.menu.setStatusIcon(true, true, false)
}

func (c *menuController) setLimit(limit int) {
	response, err := c.api.SetLimit(limit)
	if err != nil {
		logrus.WithError(err).Error(tr("Failed to set limit"))
		showAlert(tr("Failed to set limit"), response+err.Error())
	}
}

func (c *menuController) callChargeOnce(call func() (string, error), failure string) {
	response, err := call()
	if err != nil {
		logrus.WithError(err).Error(failure)
		showAlert(failure, response+err.Error())
	}
}

// chargeOnceLimitTitle labels the action that charges to the configured limit
// right away.
func chargeOnceLimitTitle(limit int) string {
	if limit <= 0 || limit >= 100 {
		return tr("Charge to Limit Now")
	}
	return fmt.Sprintf(tr("Charge to %d%% Now"), limit)
}

// chargeOnceStatusTitle shows what a running one-time charge is doing.
func chargeOnceStatusTitle(target, currentCharge int) string {
	if currentCharge <= 0 {
		return fmt.Sprintf(tr("Charging to %d%%…"), target)
	}
	return fmt.Sprintf(tr("Charging to %d%%, now %d%%"), target, currentCharge)
}

func (c *menuController) setMagSafeMode(mode config.ControlMagSafeMode) {
	if _, err := c.api.SetControlMagSafeLED(mode); err != nil {
		logrus.WithError(err).Error("Failed to set control mag safe LED")
		showAlert(tr("Failed to set MagSafe LED control"), err.Error())
	}
}

func (c *menuController) setPreventIdleSleep(checked bool) {
	if _, err := c.api.SetPreventIdleSleep(checked); err != nil {
		logrus.WithError(err).Error(tr("Failed to set prevent idle sleep"))
		showAlert(tr("Failed to set prevent idle sleep"), err.Error())
	}
}

func (c *menuController) setDisableChargingPreSleep(checked bool) {
	if _, err := c.api.SetDisableChargingPreSleep(checked); err != nil {
		logrus.WithError(err).Error("Failed to set disable charging pre sleep")
		showAlert(tr("Failed to set prevent idle sleep"), err.Error())
	}
}

func (c *menuController) setPreventSystemSleep(checked bool) {
	if _, err := c.api.SetPreventSystemSleep(checked); err != nil {
		showAlert(tr("Failed to set prevent system sleep"), err.Error())
	}
}

func (c *menuController) setPreventSleepOnAdapterDisable(checked bool) {
	if checked {
		if !showConfirmation(confirmPreventSleepOnAdapterDisable) {
			logrus.Info("User cancelled prevent sleep on adapter disable")
			c.menu.setChecked(itemPreventSleepOnAdapterDisable, false)
			return
		}
	}
	if _, err := c.api.SetPreventSleepOnAdapterDisable(checked); err != nil {
		c.menu.setChecked(itemPreventSleepOnAdapterDisable, !checked)
		showAlert(tr("Failed to set prevent sleep on adapter disable"), err.Error())
	}
}

func (c *menuController) startForceDischarge(duration time.Duration) {
	confirmation := confirmForceDischarge
	if duration == 0 {
		confirmation = confirmForceDischargeIndefinitely
	}
	if !showConfirmation(confirmation) {
		logrus.Info("User cancelled force discharge")
		return
	}
	var err error
	if duration == 0 {
		_, err = c.api.SetAdapter(false)
	} else {
		_, err = c.api.DisableAdapterFor(duration)
	}
	if err != nil {
		showAlert(tr("Failed to set force discharge"), err.Error())
	}
}

func (c *menuController) stopForceDischarge() {
	if _, err := c.api.SetAdapter(true); err != nil {
		showAlert(tr("Failed to stop force discharge"), err.Error())
	}
}

func (c *menuController) startCalibration() {
	if !showConfirmation(confirmStartCalibration) {
		logrus.Info("User cancelled auto calibration")
		return
	}
	if _, err := c.api.StartCalibration(); err != nil {
		showAlert(tr("Failed to start calibration"), err.Error())
		return
	}
	c.menu.setTitle(itemCalibrationStatus, tr("Status: In Progress"))
}

func (c *menuController) pauseCalibration() {
	if _, err := c.api.PauseCalibration(); err != nil {
		showAlert(tr("Failed to pause calibration"), err.Error())
	}
}

func (c *menuController) resumeCalibration() {
	if _, err := c.api.ResumeCalibration(); err != nil {
		showAlert(tr("Failed to resume calibration"), err.Error())
	}
}

func (c *menuController) cancelCalibration() {
	if _, err := c.api.CancelCalibration(); err != nil {
		showAlert(tr("Failed to cancel calibration"), err.Error())
		return
	}
	c.menu.setTitle(itemCalibrationStatus, tr("Status: Idle"))
}

func (c *menuController) uninstall() {
	executable, err := os.Executable()
	if err != nil {
		logrus.WithError(err).Error(tr("Failed to get executable path"))
		showAlert(tr("Failed to get executable path"), err.Error())
		return
	}
	if err := uninstallDaemon(executable); err != nil {
		logrus.WithError(err).Error(tr("Failed to uninstall daemon"))
		showAlert(tr("Failed to uninstall daemon"), err.Error())
		return
	}
	if err := UnregisterLoginItem(); err != nil {
		logrus.WithError(err).Error(tr("Failed to unregister login item"))
		showAlert(tr("Failed to unregister login item"), err.Error())
		return
	}
	c.menu.setStatusIcon(false, true, false)
}

func (c *menuController) disableLimit() {
	response, err := c.api.SetLimit(100)
	if err != nil && !pkgerrors.Is(err, client.ErrDaemonNotRunning) {
		showAlert(tr("Failed to disable charge limit"), response+err.Error())
	}
}

func (c *menuController) disableLimitFor(duration time.Duration) {
	response, err := c.api.DisableFor(duration)
	if err != nil && !pkgerrors.Is(err, client.ErrDaemonNotRunning) {
		showAlert(tr("Failed to disable charge limit"), response+err.Error())
	}
}

func temporaryDisableDuration(item menuItem) time.Duration {
	switch item {
	case itemForceDischarge1Hour:
		return time.Hour
	case itemForceDischarge2Hours:
		return 2 * time.Hour
	case itemForceDischarge4Hours:
		return 4 * time.Hour
	case itemForceDischarge8Hours:
		return 8 * time.Hour
	case itemDisableLimit1Hour:
		return time.Hour
	case itemDisableLimit2Hours:
		return 2 * time.Hour
	case itemDisableLimit4Hours:
		return 4 * time.Hour
	case itemDisableLimit8Hours:
		return 8 * time.Hour
	case itemDisableLimit12Hours:
		return 12 * time.Hour
	case itemDisableLimit24Hours:
		return 24 * time.Hour
	case itemDisableLimit2Days:
		return 2 * 24 * time.Hour
	case itemDisableLimit3Days:
		return 3 * 24 * time.Hour
	case itemDisableLimit7Days:
		return 7 * 24 * time.Hour
	default:
		panic("not a temporary disable menu item")
	}
}

func temporaryAdapterDisableCountdownTitle(remaining time.Duration) string {
	if remaining <= 0 {
		return tr("Restoring power adapter…")
	}
	return tr("Restores power adapter in ") + formatTemporaryDisableRemaining(remaining)
}

func temporaryDisableCountdownTitle(limit int, remaining time.Duration) string {
	if remaining <= 0 {
		return fmt.Sprintf(tr("Restoring %d%% limit…"), limit)
	}

	return fmt.Sprintf(tr("Restores to %d%% in %s"), limit, formatTemporaryDisableRemaining(remaining))
}

func formatTemporaryDisableRemaining(remaining time.Duration) string {
	totalMinutes := int64(remaining / time.Minute)
	if remaining%time.Minute != 0 {
		totalMinutes++
	}

	days := totalMinutes / (24 * 60)
	hours := totalMinutes % (24 * 60) / 60
	minutes := totalMinutes % 60
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf(tr("%dd"), days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf(tr("%dh"), hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf(tr("%dm"), minutes))
	}

	return strings.Join(parts, " ")
}

func (c *menuController) quit() {
	if c.eventCancel != nil {
		logrus.Debug("Cancelling event subscription")
		c.eventCancel()
	}
	logrus.Info("Quitting client")
	terminateNativeApp()
}
