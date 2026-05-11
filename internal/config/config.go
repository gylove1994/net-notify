package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gylove1994/net-notify/internal/policy"
)

// GroupEntry is one named probe group in JSON config.
type GroupEntry struct {
	Name       string   `json:"name"`
	URLs       []string `json:"urls"`
	NotifyWhen string   `json:"notify_when"`
}

// EffectiveName returns the JSON name if non-empty (trimmed), otherwise group%d for index i.
func (e GroupEntry) EffectiveName(i int) string {
	s := strings.TrimSpace(e.Name)
	if s != "" {
		return s
	}
	return fmt.Sprintf("group%d", i)
}

// File matches config.example.json fields.
type File struct {
	Interval        string       `json:"interval"`
	Timeout         string       `json:"timeout"`
	URLs            []string     `json:"urls"`
	Groups          []GroupEntry `json:"groups"`
	AlertCooldown   string       `json:"alert_cooldown"`
	NotifyTimeoutMs int          `json:"notify_timeout_ms"`
	NotifyIcon      string       `json:"notify_icon"`
	NotifyApp       string       `json:"notify_app"`
	NotifyBackend   string       `json:"notify_backend"`
	DMSPath         string       `json:"dms_path"`
	Verbose         bool         `json:"verbose"`
	NotifyUrgency   string       `json:"notify_urgency"`
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
	if err := ValidateFile(f); err != nil {
		return f, err
	}
	return f, nil
}

// ValidateFile checks structural rules (e.g. urls vs groups).
func ValidateFile(f File) error {
	if len(f.Groups) > 0 && len(f.URLs) > 0 {
		return fmt.Errorf("config: cannot set both \"urls\" and \"groups\"")
	}
	for i, g := range f.Groups {
		if len(g.URLs) == 0 {
			return fmt.Errorf("config: groups[%d] must have at least one url", i)
		}
		if _, err := policy.ParseWhen(g.NotifyWhen); err != nil {
			return fmt.Errorf("config: groups[%d] notify_when: %w", i, err)
		}
	}
	return nil
}

// LayoutFromGroups builds a flattened URL list (deduped, first-seen order) and policy groups.
func LayoutFromGroups(entries []GroupEntry) (policy.Layout, error) {
	if len(entries) == 0 {
		return policy.Layout{}, fmt.Errorf("config: groups is empty")
	}
	var flat []string
	seen := make(map[string]struct{})
	add := func(u string) {
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		flat = append(flat, u)
	}
	var groups []policy.Group
	for i, e := range entries {
		if len(e.URLs) == 0 {
			return policy.Layout{}, fmt.Errorf("config: groups[%d] must have at least one url", i)
		}
		when, err := policy.ParseWhen(e.NotifyWhen)
		if err != nil {
			return policy.Layout{}, fmt.Errorf("config: groups[%d] notify_when: %w", i, err)
		}
		name := e.EffectiveName(i)
		gu := append([]string(nil), e.URLs...)
		for _, u := range gu {
			add(u)
		}
		groups = append(groups, policy.Group{Name: name, When: when, URLs: gu})
	}
	return policy.Layout{FlatURLs: flat, Groups: groups}, nil
}

// ParseDuration returns d if s is empty, otherwise parses s.
func ParseDuration(s string, d time.Duration) (time.Duration, error) {
	if s == "" {
		return d, nil
	}
	return time.ParseDuration(s)
}

// Save writes f to path as indented JSON (validates first). Uses a temp file in the same directory and rename.
func Save(path string, f File) error {
	if err := ValidateFile(f); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("config save: %w", err)
	}
	b = append(b, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".net-notify-config-*.tmp")
	if err != nil {
		return fmt.Errorf("config save: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		return fmt.Errorf("config save: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("config save: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config save: %w", err)
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode() & 0o777
	}
	_ = os.Chmod(tmpPath, mode)
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("config save: %w", err)
	}
	removeTemp = false
	return nil
}

// UpdateGroupName sets groups[index].name in the JSON file at path and saves. Index is 0-based.
func UpdateGroupName(path string, index int, newName string) error {
	f, err := Load(path)
	if err != nil {
		return err
	}
	if len(f.Groups) == 0 {
		return fmt.Errorf("config: no \"groups\" in file (flat \"urls\" layout has no group names to edit)")
	}
	if index < 0 || index >= len(f.Groups) {
		return fmt.Errorf("groups: index %d out of range [0, %d]", index, len(f.Groups)-1)
	}
	f.Groups[index].Name = newName
	return Save(path, f)
}
