package state

import "time"

// Cooldown suppresses repeated failure notifications while still alerting on transitions.
type Cooldown struct {
	Cooldown time.Duration

	prevFail  bool
	lastAlert time.Time
}

// ShouldNotify returns whether to send a notification for this failing round.
// When failing is false, internal state is reset to "healthy".
func (c *Cooldown) ShouldNotify(now time.Time, failing bool) bool {
	if !failing {
		c.prevFail = false
		return false
	}
	if !c.prevFail {
		c.prevFail = true
		c.lastAlert = now
		return true
	}
	if c.Cooldown <= 0 {
		return true
	}
	if now.Sub(c.lastAlert) >= c.Cooldown {
		c.lastAlert = now
		return true
	}
	return false
}
