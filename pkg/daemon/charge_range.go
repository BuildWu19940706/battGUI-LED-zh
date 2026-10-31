package daemon

import (
	"fmt"
	"net/http"

	"github.com/charlie0129/batt/pkg/compatibility"
	"github.com/gin-gonic/gin"
)

// setChargeRange commits a pair, rather than two independent client requests.
// The transition lock also protects the timer and one-time-charge snapshot.
func setChargeRange(c *gin.Context) {
	if !requireCapability(c, compatibility.FeatureLowerLimit) {
		return
	}
	var input struct {
		Upper *int `json:"upper"`
		Lower *int `json:"lower"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.Upper == nil || input.Lower == nil {
		c.IndentedJSON(http.StatusBadRequest, "upper and lower must be integer percentages")
		return
	}
	upper, lower := *input.Upper, *input.Lower
	if upper < 10 || upper > 100 || lower < 10 || lower >= upper {
		c.IndentedJSON(http.StatusBadRequest, "range must satisfy 10 <= lower < upper <= 100")
		return
	}
	caps := getCapabilities()
	if !caps.SupportsLimit(upper) {
		c.IndentedJSON(http.StatusBadRequest, fmt.Sprintf("this Mac only offers charge limits of %s", compatibility.FormatLimits(caps.SupportedLimits)))
		return
	}
	if caps.ChargeControlMode == compatibility.ChargeControlAdapter && upper < 100 && upper-lower < adapterMinBand {
		c.IndentedJSON(http.StatusBadRequest, "adapter mode requires at least a 5% range")
		return
	}

	chargeControlTransitionMu.Lock()
	defer chargeControlTransitionMu.Unlock()
	if calibrationOwnsChargeLimit() {
		c.IndentedJSON(http.StatusBadRequest, ErrCalibrationControlsChargeLimit.Error())
		return
	}

	maintainLoopInnerLock.Lock()
	oldUpper, oldLower := conf.UpperLimit(), conf.LowerLimit()
	oldUntil, oldPre := conf.DisableUntil(), conf.PreDisableLimit()
	oldTarget := conf.ChargeOnceTarget()
	restore := func() {
		conf.SetChargeRange(oldUpper, oldLower)
		conf.ClearDisableTimer()
		if !oldUntil.IsZero() {
			conf.SetDisableTimer(oldUntil, oldPre)
		}
		if oldTarget == 0 {
			conf.ClearChargeOnceTarget()
		} else {
			conf.SetChargeOnceTarget(oldTarget)
		}
	}
	conf.SetChargeRange(upper, lower)
	conf.ClearDisableTimer()
	conf.ClearChargeOnceTarget()
	if err := conf.Save(); err != nil {
		restore()
		maintainLoopInnerLock.Unlock()
		c.IndentedJSON(http.StatusInternalServerError, fmt.Sprintf("could not save charge range: %v", err))
		return
	}
	maintainLoopInnerLock.Unlock()
	invalidateControlStatus()
	if !maintainLoopForced() {
		maintainLoopInnerLock.Lock()
		restore()
		saveErr := conf.Save()
		maintainLoopInnerLock.Unlock()
		restored := maintainLoopForced()
		c.IndentedJSON(http.StatusInternalServerError, fmt.Sprintf("could not apply charge range; previous settings restored (save error: %v, hardware restored: %t)", saveErr, restored))
		return
	}
	reportSupersededChargeOnce(oldTarget, "charge range changed")
	c.IndentedJSON(http.StatusCreated, fmt.Sprintf("charge range set to %d%%–%d%%", lower, upper))
}
