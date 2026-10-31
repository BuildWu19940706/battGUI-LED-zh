package daemon

import (
	"github.com/charlie0129/batt/pkg/smc"
	"github.com/sirupsen/logrus"
	"sync"
)

// One writer owns all daemon LED writes, including sleep and shutdown release.
// A failed write/readback is terminal until the daemon is restarted.
type ledController struct {
	mu                   sync.Mutex
	failed, owned, known bool
	last                 smc.MagSafeLedState
}

func (c *ledController) apply(state smc.MagSafeLedState, available bool, write func(smc.MagSafeLedState) error, read func() (smc.MagSafeLedState, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed {
		return
	}
	if !available && !c.owned {
		return
	}
	if !available {
		state = smc.LEDSystem
	}
	if c.known && c.last == state {
		return
	}
	// Never take ownership just to write the system's existing default.
	if state == smc.LEDSystem && !c.owned {
		return
	}
	err := write(state)
	if err == nil && state != smc.LEDSystem && read != nil {
		var got smc.MagSafeLedState
		got, err = read()
		if err == nil && got != state {
			err = errLEDReadback
		}
	}
	if err != nil {
		logrus.WithError(err).Warn("MagSafe control failed; restoring system control for this session")
		if restoreErr := write(smc.LEDSystem); restoreErr != nil {
			logrus.WithError(restoreErr).Error("could not restore system MagSafe control")
		}
		c.failed = true
		c.owned = false
		c.known = false
		return
	}
	c.last, c.known, c.owned = state, true, state != smc.LEDSystem
}

func (c *ledController) hasFailed() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.failed }
