package notify

import (
	"context"
	"os/exec"
	"strconv"
)

// NotifySend invokes notify-send with Freedesktop urgency hints (-u low|normal|critical).
// DankMaterialShell exposes a NotificationServer; notify-send usually reaches the same pipeline
// so urgency affects DMS timeouts/rules (e.g. notificationTimeoutCritical) and popup styling.
type NotifySend struct {
	Urgency   string
	App       string
	Icon      string
	TimeoutMs int
}

func (n *NotifySend) Notify(ctx context.Context, summary, body string) error {
	u := NormalizeUrgency(n.Urgency)
	app := n.App
	if app == "" {
		app = "net-notify"
	}
	ms := n.TimeoutMs
	if ms <= 0 {
		ms = 30000
	}
	args := []string{"notify-send", "-u", u, "-a", app, "-t", strconv.Itoa(ms)}
	if n.Icon != "" {
		args = append(args, "-i", n.Icon)
	}
	args = append(args, summary, body)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	return cmd.Run()
}
