package powerinfo

import "time"

// ControlStatus is optional additive telemetry; reasons are stable identifiers,
// not localized daemon messages. A GUI may render them in its own language.
type ControlStatus struct {
	TargetPercent int       `json:"targetPercent"`
	LowerPercent  int       `json:"lowerPercent"`
	ChargePercent int       `json:"chargePercent"`
	State         string    `json:"state"`
	Reason        string    `json:"reason"`
	SampledAt     time.Time `json:"sampledAt"`
	LED           LEDStatus `json:"led"`
}

type LEDStatus struct {
	InterfacePresent bool   `json:"interfacePresent"`
	Available        bool   `json:"available"`
	Reason           string `json:"reason,omitempty"`
	Mode             string `json:"mode"`
	Desired          string `json:"desired"`
}

type ControlInput struct {
	Valid, PluggedIn, Charging, Full, Discharging bool
	ForcedDischarge, Calibrating, Suspended       bool
	Charge, Target, Lower, ConfiguredUpper        int
	Now                                           time.Time
}

// ControlTracker only keeps evidence from the current uninterrupted session.
// Its owner serializes calls; Reset is required on wake and configuration reload.
type ControlTracker struct {
	reached  bool
	previous ControlInput
}

func (t *ControlTracker) Reset() { *t = ControlTracker{} }

func (t *ControlTracker) Update(in ControlInput) ControlStatus {
	last := t.previous
	if !in.Valid || !in.PluggedIn || in.Suspended || in.Target != last.Target || in.Lower != last.Lower || in.ConfiguredUpper != last.ConfiguredUpper || in.Now.Sub(last.Now) > 30*time.Second || in.Now.Before(last.Now) {
		t.reached = false
	}
	t.previous = in
	out := ControlStatus{TargetPercent: in.Target, LowerPercent: in.Lower, ChargePercent: in.Charge, SampledAt: in.Now, State: "unknown", Reason: "data_unavailable"}
	switch {
	case !in.Valid || in.Charge < 0 || in.Charge > 100 || in.Target < 10 || in.Target > 100:
		t.reached = false
	case in.Suspended:
		out.State, out.Reason = "sleeping", "sleeping"
	case !in.PluggedIn:
		out.State, out.Reason = "on_battery", "unplugged"
	case in.Calibrating:
		t.reached = false
		out.State, out.Reason = "calibrating", "calibration"
	case in.ForcedDischarge:
		t.reached = false
		out.State, out.Reason = "discharging", "forced_discharge"
	case in.Charging:
		t.reached = false
		out.State, out.Reason = "charging", "charging_to_target"
	case in.Charge >= in.Target && (in.Target < 100 || in.Full):
		t.reached = true
		out.State, out.Reason = "reached", "target_reached"
	case t.reached && in.Target < 100 && in.Charge >= in.Lower:
		out.State, out.Reason = "holding", "holding_range"
	case in.Discharging:
		t.reached = false
		out.State, out.Reason = "discharging", "battery_supplementing"
	default:
		t.reached = false
		out.State, out.Reason = "paused", "pause_unconfirmed"
	}
	return out
}
