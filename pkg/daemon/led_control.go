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
	releasePending       bool
	releaseAttempts      int
	last                 smc.MagSafeLedState
}

func (c *ledController) apply(state smc.MagSafeLedState, available bool, write func(smc.MagSafeLedState) error, read func() (smc.MagSafeLedState, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed {
		if c.releasePending && c.releaseAttempts < 3 {
			c.release(write)
		}
		return
	}
	if !available && !c.owned {
		return
	}
	if !available {
		state = smc.LEDSystem
	}
	// Recover ownership left behind by a crashed process. Only verified-capable
	// hardware reaches this branch; firmware-unverified devices remain untouched.
	if !c.known {
		if err := write(smc.LEDSystem); err != nil {
			c.failed, c.owned, c.releasePending = true, true, true
			c.releaseAttempts = 1
			return
		}
		c.known, c.last = true, smc.LEDSystem
	}
	if c.known && c.last == state {
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
		c.failed = true
		c.owned, c.releasePending = true, true
		c.release(write)
		return
	}
	c.last, c.known, c.owned = state, true, state != smc.LEDSystem
}

func (c *ledController) release(write func(smc.MagSafeLedState) error) {
	c.releaseAttempts++
	if err := write(smc.LEDSystem); err != nil {
		logrus.WithError(err).Error("could not restore system MagSafe control")
		return
	}
	c.owned, c.releasePending, c.known, c.last = false, false, true, smc.LEDSystem
}

func (c *ledController) hasPendingRelease() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.releasePending
}

func (c *ledController) hasFailed() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.failed }
