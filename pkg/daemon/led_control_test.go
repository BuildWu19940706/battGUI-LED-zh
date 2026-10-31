package daemon

import (
	"errors"
	"github.com/charlie0129/batt/pkg/smc"
	"testing"
)

func TestLEDWritesAreDeduplicatedAndRelease(t *testing.T) {
	c := ledController{}
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
		c := ledController{}
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

func TestLEDUnsupportedDoesNotWrite(t *testing.T) {
	c := ledController{}
	c.apply(smc.LEDGreen, false, func(s smc.MagSafeLedState) error { t.Fatal("unverified hardware written"); return nil }, nil)
}
