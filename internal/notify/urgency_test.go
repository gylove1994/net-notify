package notify

import "testing"

func TestNormalizeUrgency(t *testing.T) {
	t.Parallel()
	if got := NormalizeUrgency(""); got != UrgencyCritical {
		t.Fatalf("empty: got %q", got)
	}
	if got := NormalizeUrgency("LOW"); got != UrgencyLow {
		t.Fatalf("low: got %q", got)
	}
	if got := NormalizeUrgency("Normal"); got != UrgencyNormal {
		t.Fatalf("normal: got %q", got)
	}
	if got := NormalizeUrgency("critical"); got != UrgencyCritical {
		t.Fatalf("critical: got %q", got)
	}
	if got := NormalizeUrgency("weird"); got != UrgencyCritical {
		t.Fatalf("unknown: got %q", got)
	}
}
