package config

import (
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
