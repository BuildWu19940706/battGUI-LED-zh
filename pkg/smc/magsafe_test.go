package smc

import (
	"github.com/charlie0129/gosmc"
	"testing"
)

func TestMagSafeSystemAndMalformedRead(t *testing.T) {
	for _, data := range [][]byte{{0}, {0, 1}} {
		v, err := gosmc.NewValue(MagSafeLedKey, gosmc.TypeUInt8, data)
		if err != nil {
			t.Fatal(err)
		}
		c := NewMockValues(v)
		if err := c.Open(); err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		got, err := c.GetMagSafeLedState()
		if len(data) == 1 && (got != LEDSystem || err != nil) {
			t.Fatalf("system state=%v error=%v", got, err)
		}
		if len(data) != 1 && err == nil {
			t.Fatal("malformed LED read reported success")
		}
	}
}
