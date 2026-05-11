package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFile_BothURLsAndGroups(t *testing.T) {
	err := ValidateFile(File{
		URLs:   []string{"https://a"},
		Groups: []GroupEntry{{Name: "g", URLs: []string{"https://b"}, NotifyWhen: "any_fail"}},
	})
	if err == nil || !strings.Contains(err.Error(), "urls") {
		t.Fatalf("expected mutual exclusion error, got %v", err)
	}
}

func TestLayoutFromGroups_Dedupe(t *testing.T) {
	layout, err := LayoutFromGroups([]GroupEntry{
		{Name: "a", URLs: []string{"https://x", "https://y"}, NotifyWhen: "all_fail"},
		{Name: "b", URLs: []string{"https://y", "https://z"}, NotifyWhen: "any_fail"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.FlatURLs) != 3 {
		t.Fatalf("flat urls: %+v", layout.FlatURLs)
	}
}

func TestGroupEntry_EffectiveName(t *testing.T) {
	g := GroupEntry{Name: "  x  ", URLs: []string{"https://a"}, NotifyWhen: "any_fail"}
	if g.EffectiveName(3) != "x" {
		t.Fatalf("%q", g.EffectiveName(3))
	}
	g = GroupEntry{Name: "", URLs: []string{"https://a"}, NotifyWhen: "any_fail"}
	if g.EffectiveName(2) != "group2" {
		t.Fatalf("%q", g.EffectiveName(2))
	}
}

func TestSaveAndUpdateGroupName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	f := File{
		Interval: "1m",
		Groups: []GroupEntry{
			{Name: "old", URLs: []string{"https://x"}, NotifyWhen: "any_fail"},
			{Name: "b", URLs: []string{"https://z"}, NotifyWhen: "all_fail"},
		},
	}
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	f2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f2.Groups[0].Name != "old" {
		t.Fatalf("%+v", f2.Groups[0])
	}
	if err := UpdateGroupName(path, 0, "home"); err != nil {
		t.Fatal(err)
	}
	f3, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f3.Groups[0].Name != "home" || f3.Groups[1].Name != "b" {
		t.Fatalf("%+v", f3.Groups)
	}
	if err := UpdateGroupName(path, 99, "x"); err == nil {
		t.Fatal("expected range error")
	}
}
