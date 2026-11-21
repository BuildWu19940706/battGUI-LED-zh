package daemon

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/charlie0129/batt/pkg/compatibility"
)

// setChargeRange commits both thresholds and supersedes any temporary limit
// as one transition. A failed hardware write restores the previous workflow.
func (d *Daemon) setChargeRange(w http.ResponseWriter, r *http.Request) {
	if !d.requireCapability(w, compatibility.FeatureLowerLimit) {
		return
	}
	var input struct {
		Upper *int `json:"upper"`
		Lower *int `json:"lower"`
	}
	if err := decodeJSON(r, &input); err != nil || input.Upper == nil || input.Lower == nil {
		d.fail(w, http.StatusBadRequest, errors.New("upper and lower must be integer percentages"))
		return
	}

	d.transitionMu.Lock()
	defer d.transitionMu.Unlock()
	if !d.requireCapability(w, compatibility.FeatureLowerLimit) {
		return
	}
	upper, lower := *input.Upper, *input.Lower
	if upper > 100 || lower < 10 || lower >= upper {
		d.fail(w, http.StatusBadRequest, errors.New("range must satisfy 10 <= lower < upper <= 100"))
		return
	}
	caps := d.getCapabilities()
	if !caps.SupportsLimit(upper) {
		d.fail(w, http.StatusBadRequest, fmt.Errorf("this Mac only offers charge limits of %s", compatibility.FormatLimits(caps.SupportedLimits)))
		return
	}
	if caps.ChargeControlMode == compatibility.ChargeControlAdapter && upper < 100 && upper-lower < adapterMinBand {
		d.fail(w, http.StatusBadRequest, fmt.Errorf("adapter mode requires at least a %d%% range", adapterMinBand))
		return
	}

	d.loopMu.Lock()
	defer d.loopMu.Unlock()
	if d.calibrationOwnsChargeLimit() {
		d.fail(w, http.StatusBadRequest, ErrCalibrationControlsChargeLimit)
		return
	}
	conf := d.conf
	oldUpper, oldLower := conf.UpperLimit(), conf.LowerLimit()
	oldUntil, oldPre := conf.DisableUntil(), conf.PreDisableLimit()
	oldTarget := d.supersedeChargeOnce()
	conf.SetChargeRange(upper, lower)
	conf.ClearDisableTimer()
	restore := func() {
		conf.SetChargeRange(oldUpper, oldLower)
		if !oldUntil.IsZero() {
			conf.SetDisableTimer(oldUntil, oldPre)
		}
		if oldTarget > 0 {
			conf.SetChargeOnceTarget(oldTarget)
		}
	}
	if err := conf.Save(); err != nil {
		restore()
		d.fail(w, http.StatusInternalServerError, fmt.Errorf("could not save charge range: %w", err))
		return
	}
	d.invalidateControlStatus()
	if !d.maintainLoopLocked(true) {
		restore()
		err := errors.New("could not apply charge range")
		if saveErr := conf.Save(); saveErr != nil {
			err = errors.Join(err, fmt.Errorf("could not save previous settings: %w", saveErr))
		}
		if !d.maintainLoopLocked(true) {
			err = errors.Join(err, errors.New("could not reapply previous hardware settings"))
		}
		d.fail(w, http.StatusInternalServerError, err)
		return
	}
	d.reportSupersededChargeOnce(oldTarget, "charge range changed")
	writeJSON(w, http.StatusCreated, fmt.Sprintf("charge range set to %d%%–%d%%", lower, upper))
}
