package notify

import (
	"context"
	"strings"
	"unicode/utf8"
)

// Notifier sends a desktop notification.
type Notifier interface {
	Notify(ctx context.Context, summary, body string) error
}

const (
	BackendDMS        = "dms"
	BackendNotifySend = "notify-send"
)

// TruncateSummary limits summary length for notification servers.
func TruncateSummary(s string, maxRunes int) string {
	return truncateRunes(s, maxRunes)
}

// TruncateBody limits body length.
func TruncateBody(s string, maxRunes int) string {
	return truncateRunes(s, maxRunes)
}

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	if len(runes) >= maxRunes {
		return string(runes[:maxRunes-1]) + "…"
	}
	return s
}

// BackendName normalizes user/config backend string.
func BackendName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case BackendNotifySend, "notifysend":
		return BackendNotifySend
	default:
		return BackendDMS
	}
}
