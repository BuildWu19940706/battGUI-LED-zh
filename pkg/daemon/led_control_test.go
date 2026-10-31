package daemon

import (
	"errors"
	"github.com/charlie0129/batt/pkg/smc"
	"testing"
)

func TestLEDWritesAreDeduplicatedAndRelease(t *testing.T) {
	c := ledController{known: true, last: smc.LEDSystem}
	state := smc.LEDSystem
	writes := []smc.MagSafeLedState{}
	write := func(s smc.MagSafeLedState) error { state = s; writes = append(writes, s); return nil }
	read := func() (smc.MagSafeLedState, error) { return state, nil }
	c.apply(smc.LEDGreen, true, write, read)
	c.apply(smc.LEDGreen, true, write, read)
	if len(writes) != 1 {
		t.Fatalf("unchanged state rewrote LED: %v", writes)
	}
	c.apply(smc.LEDSystem, true, write, read)
	if len(writes) != 2 || state != smc.LEDSystem {
		t.Fatalf("release failed: %v", writes)
	}
}

func TestLEDFailureRestoresSystemAndDisablesSession(t *testing.T) {
	for _, readFailure := range []bool{false, true} {
		c := ledController{known: true, last: smc.LEDSystem}
		writes := []smc.MagSafeLedState{}
		write := func(s smc.MagSafeLedState) error {
			writes = append(writes, s)
			if !readFailure && s != smc.LEDSystem {
				return errors.New("write denied")
			}
			return nil
		}
		read := func() (smc.MagSafeLedState, error) { return smc.LEDSystem, errors.New("read denied") }
		c.apply(smc.LEDGreen, true, write, read)
		c.apply(smc.LEDOrange, true, write, read)
		if !c.failed || len(writes) != 2 || writes[1] != smc.LEDSystem {
			t.Fatalf("no fail-to-system: failed=%v writes=%v", c.failed, writes)
		}
	}
}

func TestLEDRestartEstablishesSystemBaseline(t *testing.T) {
	c := ledController{}
	state := smc.LEDGreen
	c.apply(smc.LEDSystem, true, func(s smc.MagSafeLedState) error { state = s; return nil }, func() (smc.MagSafeLedState, error) { return state, nil })
	if state != smc.LEDSystem {
		t.Fatal("fresh daemon left a previous process's green LED active")
	}
}

func TestLEDFailedReleaseCanRecoverWithoutColors(t *testing.T) {
	c := ledController{known: true, owned: true, last: smc.LEDGreen}
	denyRelease := true
	state := smc.LEDGreen
	write := func(s smc.MagSafeLedState) error {
		if s != smc.LEDSystem || denyRelease {
			return errors.New("SMC temporarily unavailable")
		}
		state = s
		return nil
	}
	c.apply(smc.LEDOrange, true, write, nil)
	if !c.hasPendingRelease() {
		t.Fatal("failed release abandoned hardware ownership")
	}
	denyRelease = false
	c.apply(smc.LEDGreen, true, write, nil)
	if state != smc.LEDSystem || c.hasPendingRelease() || !c.hasFailed() {
		t.Fatal("failed controller did not recover system-only control")
	}
}

func TestLEDUnsupportedDoesNotWrite(t *testing.T) {
	c := ledController{}
	c.apply(smc.LEDGreen, false, func(s smc.MagSafeLedState) error { t.Fatal("unverified hardware written"); return nil }, nil)
}
