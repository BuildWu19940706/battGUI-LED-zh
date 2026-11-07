package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/charlie0129/batt/pkg/smc"
	"github.com/sirupsen/logrus"
)

// One writer owns all daemon LED writes, including sleep and shutdown release.
// A failed session only resumes after an explicit settings request successfully
// establishes a system-controlled baseline, or after daemon restart.
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
		// Firmware can acknowledge the write before its readback changes. The
		// verified M4 returned the old color at 0-11ms and the new one at 50ms.
		// Retry reads only, with at most 500ms of settling waits; never rewrite
		// colors here. Actual read errors still fail immediately.
		for _, delay := range [...]time.Duration{0, 25 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond, 150 * time.Millisecond, 250 * time.Millisecond} {
			if delay > 0 {
				time.Sleep(delay)
			}
			var got smc.MagSafeLedState
			got, err = read()
			if err != nil || got == state {
				break
			}
			err = fmt.Errorf("%w: requested=%d readback=%d", errLEDReadback, state, got)
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

func (c *ledController) release(write func(smc.MagSafeLedState) error) error {
	c.releaseAttempts++
	if err := write(smc.LEDSystem); err != nil {
		logrus.WithError(err).Error("could not restore system MagSafe control")
		return err
	}
	c.owned, c.releasePending, c.known, c.last = false, false, true, smc.LEDSystem
	return nil
}

// retry is called only for an explicit, validated user request. A failed
// release keeps the session failed; it never authorizes a colored write.
func (c *ledController) retry(write func(smc.MagSafeLedState) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.failed {
		return nil
	}
	c.releaseAttempts = 0
	c.owned, c.releasePending = true, true
	if err := c.release(write); err != nil {
		return fmt.Errorf("could not restore system MagSafe control before retry: %w", err)
	}
	c.failed, c.releaseAttempts = false, 0
	return nil
}

func (c *ledController) failureState() (failed, releasePending bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failed, c.releasePending
}

func (c *ledController) hasPendingRelease() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.releasePending
}

func (c *ledController) hasFailed() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.failed }
