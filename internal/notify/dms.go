package notify

import (
	"context"
	"path/filepath"
	"strconv"

	"os/exec"
)

// DMS sends notifications via `dms notify` (DankMaterialShell).
type DMS struct {
	Path      string
	App       string
	TimeoutMs int
	Icon      string
}

func (d *DMS) Notify(ctx context.Context, summary, body string) error {
	app := d.App
	if app == "" {
		app = "net-notify"
	}
	ms := d.TimeoutMs
	if ms <= 0 {
		ms = 30000
	}
	bin := d.Path
	if bin == "" {
		bin = "dms"
	} else {
		bin = filepath.Clean(bin)
	}
	args := []string{"notify", summary, body, "--app", app, "--timeout", strconv.Itoa(ms)}
	if d.Icon != "" {
		args = append(args, "--icon", d.Icon)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	return cmd.Run()
}
