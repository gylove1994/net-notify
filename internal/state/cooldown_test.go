package state

import (
	"testing"
	"time"
)

func TestCooldown_TransitionAndRepeat(t *testing.T) {
	cd := &Cooldown{Cooldown: time.Hour}
	t0 := time.Unix(1_000_000, 0)

	if cd.ShouldNotify(t0, false) {
		t.Fatal("healthy should not notify")
	}
	if !cd.ShouldNotify(t0.Add(time.Second), true) {
		t.Fatal("first failure should notify")
	}
	if cd.ShouldNotify(t0.Add(2*time.Second), true) {
		t.Fatal("still within cooldown")
	}
	if !cd.ShouldNotify(t0.Add(time.Hour+time.Second), true) {
		t.Fatal("after cooldown should notify again")
	}
}

func TestCooldown_Recovery(t *testing.T) {
	cd := &Cooldown{Cooldown: time.Minute}
	t0 := time.Unix(2_000_000, 0)
	if !cd.ShouldNotify(t0, true) {
		t.Fatal("first fail")
	}
	if cd.ShouldNotify(t0.Add(time.Second), true) {
		t.Fatal("cooldown")
	}
	if cd.ShouldNotify(t0.Add(10*time.Second), false) {
		t.Fatal("recovery should not notify")
	}
	if !cd.ShouldNotify(t0.Add(20*time.Second), true) {
		t.Fatal("fail after recovery should notify (transition)")
	}
}

func TestCooldown_ZeroMeansRepeat(t *testing.T) {
	cd := &Cooldown{Cooldown: 0}
	t0 := time.Unix(3_000_000, 0)
	if !cd.ShouldNotify(t0, true) || !cd.ShouldNotify(t0.Add(time.Millisecond), true) {
		t.Fatal("zero cooldown should allow repeat")
	}
}
