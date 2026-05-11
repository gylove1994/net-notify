package notify

import (
	"context"
	"os/exec"
)

// NotifySend uses `notify-send -u critical` for highest desktop urgency where supported.
type NotifySend struct{}

func (NotifySend) Notify(ctx context.Context, summary, body string) error {
	cmd := exec.CommandContext(ctx, "notify-send", "-u", "critical", summary, body)
	return cmd.Run()
}
