package powerinfo

import (
	"testing"
	"time"
)

func TestControlTargetAndHold(t *testing.T) {
	tracker := ControlTracker{}
	in := ControlInput{Valid: true, PluggedIn: true, Target: 80, Lower: 78, Charge: 79, Now: time.Unix(100, 0)}
	if got := tracker.Update(in); got.State != "paused" {
		t.Fatalf("a paused 79%% battery with no history is not reached: %+v", got)
	}
	in.Charging = true
	if got := tracker.Update(in); got.State != "charging" {
		t.Fatal(got)
	}
	in.Charge = 80
	in.Charging = false
	if got := tracker.Update(in); got.State != "reached" {
		t.Fatal(got)
	}
	in.Charge = 78
	if got := tracker.Update(in); got.State != "holding" {
		t.Fatal(got)
	}
	in.Charge = 77
	if got := tracker.Update(in); got.State != "paused" {
		t.Fatal(got)
	}
}

func TestControlInvalidatesReachedHistory(t *testing.T) {
	cases := []struct {
		name   string
		change func(*ControlInput)
	}{
		{"unplug", func(in *ControlInput) { in.PluggedIn = false }},
		{"sleep", func(in *ControlInput) { in.Suspended = true }},
		{"failed sample", func(in *ControlInput) { in.Valid = false }},
		{"changed target", func(in *ControlInput) { in.Target = 90 }},
		{"changed lower", func(in *ControlInput) { in.Lower = 75 }},
		{"stale", func(in *ControlInput) { in.Now = in.Now.Add(time.Minute) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracker := ControlTracker{}
			in := ControlInput{Valid: true, PluggedIn: true, Target: 80, Lower: 78, Charge: 80, Now: time.Unix(100, 0)}
			tracker.Update(in)
			in.Charge = 79
			tc.change(&in)
			tracker.Update(in)
			in = ControlInput{Valid: true, PluggedIn: true, Target: 80, Lower: 78, Charge: 79, Now: in.Now.Add(time.Second)}
			if got := tracker.Update(in); got.State != "paused" {
				t.Fatalf("stale green after %s: %+v", tc.name, got)
			}
		})
	}
}

func TestControlSpecialModes(t *testing.T) {
	cases := []struct {
		name string
		in   ControlInput
		want string
	}{
		{"one time full still charging", ControlInput{Target: 100, Charge: 80, Charging: true}, "charging"},
		{"100 percent is not proof of full", ControlInput{Target: 100, Charge: 100}, "paused"},
		{"confirmed full", ControlInput{Target: 100, Charge: 100, Full: true}, "reached"},
		{"discharge above target", ControlInput{Target: 80, Charge: 90, ForcedDischarge: true}, "discharging"},
		{"calibration above target", ControlInput{Target: 80, Charge: 90, Calibrating: true}, "calibrating"},
		{"invalid percentage", ControlInput{Target: 80, Charge: 101}, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			in.Valid = true
			in.PluggedIn = true
			in.Lower = 78
			in.Now = time.Now()
			tracker := ControlTracker{}
			got := tracker.Update(in)
			if got.State != tc.want {
				t.Fatalf("got %+v want %s", got, tc.want)
			}
		})
	}
}
