package daemon

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charlie0129/batt/pkg/smc"
	"github.com/sirupsen/logrus"
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

// Regression: the M4 returns the previous color for about 50ms after a
// successful write. Checking only the first read falsely disables the session.
func TestLEDDelayedReadbackDoesNotDisableSession(t *testing.T) {
	for _, target := range []smc.MagSafeLedState{smc.LEDGreen, smc.LEDOrange, smc.LEDOff} {
		t.Run(targetName(target), func(t *testing.T) {
			c := ledController{known: true, last: smc.LEDSystem}
			previous := smc.LEDOrange
			if target == smc.LEDOrange {
				previous = smc.LEDGreen
			}
			var writes []smc.MagSafeLedState
			write := func(state smc.MagSafeLedState) error {
				writes = append(writes, state)
				return nil
			}
			reads := 0
			read := func() (smc.MagSafeLedState, error) {
				reads++
				if reads < 3 {
					return previous, nil
				}
				return target, nil
			}
			c.apply(target, true, write, read)
			if c.hasFailed() || c.hasPendingRelease() || c.last != target || !c.owned {
				t.Fatalf("valid delayed color was rejected: failed=%v last=%d owned=%v", c.failed, c.last, c.owned)
			}
			if reads != 3 {
				t.Fatalf("did not stop verification at the first matching response: %d reads", reads)
			}
			c.apply(target, true, write, read)
			if len(writes) != 1 || writes[0] != target {
				t.Fatalf("readback verification rewrote the color: %v", writes)
			}
		})
	}
}

func targetName(state smc.MagSafeLedState) string {
	switch state {
	case smc.LEDGreen:
		return "green"
	case smc.LEDOrange:
		return "orange"
	default:
		return "off"
	}
}

func TestLEDPersistentMismatchRestoresSystemWithDiagnosticValues(t *testing.T) {
	var log bytes.Buffer
	logger := logrus.StandardLogger()
	previousOutput := logger.Out
	logger.SetOutput(&log)
	t.Cleanup(func() { logger.SetOutput(previousOutput) })
	c := ledController{known: true, last: smc.LEDSystem}
	var writes []smc.MagSafeLedState
	write := func(state smc.MagSafeLedState) error { writes = append(writes, state); return nil }
	reads := 0
	read := func() (smc.MagSafeLedState, error) { reads++; return smc.LEDOrange, nil }
	start := time.Now()
	c.apply(smc.LEDGreen, true, write, read)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("verification did not finish promptly: %v", elapsed)
	}
	c.apply(smc.LEDOrange, true, write, read)
	if !c.hasFailed() || c.hasPendingRelease() || len(writes) != 2 || writes[0] != smc.LEDGreen || writes[1] != smc.LEDSystem {
		t.Fatalf("persistent mismatch did not stop colors and restore system: failed=%v writes=%v", c.failed, writes)
	}
	if reads < 2 {
		t.Fatal("persistent mismatch was declared before allowing hardware to settle")
	}
	if !strings.Contains(log.String(), "requested=3") || !strings.Contains(log.String(), "readback=4") {
		t.Fatalf("failure did not identify the requested and actual colors: %s", log.String())
	}
}

func TestLEDReadErrorAfterStaleValueFailsSafely(t *testing.T) {
	c := ledController{known: true, last: smc.LEDSystem}
	var writes []smc.MagSafeLedState
	write := func(state smc.MagSafeLedState) error { writes = append(writes, state); return nil }
	reads := 0
	read := func() (smc.MagSafeLedState, error) {
		reads++
		if reads == 1 {
			return smc.LEDOrange, nil
		}
		return smc.LEDSystem, errors.New("SMC read unavailable")
	}
	c.apply(smc.LEDGreen, true, write, read)
	if reads != 2 || !c.hasFailed() || c.hasPendingRelease() || len(writes) != 2 || writes[1] != smc.LEDSystem {
		t.Fatalf("read error during settling did not fail safely: reads=%d failed=%v writes=%v", reads, c.failed, writes)
	}
}
