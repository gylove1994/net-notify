package config

import (
	"encoding/json"
	"os"
	"time"
)

// File matches config.example.json fields.
type File struct {
	Interval        string   `json:"interval"`
	Timeout         string   `json:"timeout"`
	URLs            []string `json:"urls"`
	AlertCooldown   string   `json:"alert_cooldown"`
	NotifyTimeoutMs int      `json:"notify_timeout_ms"`
	NotifyIcon      string   `json:"notify_icon"`
	NotifyApp       string   `json:"notify_app"`
	NotifyBackend   string   `json:"notify_backend"`
	DMSPath         string   `json:"dms_path"`
}

// Load reads JSON config from path.
func Load(path string) (File, error) {
	var f File
	b, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	return f, nil
}

// ParseDuration returns d if s is empty, otherwise parses s.
func ParseDuration(s string, d time.Duration) (time.Duration, error) {
	if s == "" {
		return d, nil
	}
	return time.ParseDuration(s)
}
