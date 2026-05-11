package notify

import "strings"

// Freedesktop notification urgency levels (notify-send -u).
const (
	UrgencyLow      = "low"
	UrgencyNormal   = "normal"
	UrgencyCritical = "critical"
)

// NormalizeUrgency maps config/CLI input to low | normal | critical.
// Empty string defaults to critical so outage alerts match the highest tier unless overridden.
func NormalizeUrgency(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case UrgencyLow:
		return UrgencyLow
	case UrgencyNormal:
		return UrgencyNormal
	case UrgencyCritical, "":
		return UrgencyCritical
	default:
		return UrgencyCritical
	}
}
