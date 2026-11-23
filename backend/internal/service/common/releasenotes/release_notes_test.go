package releasenotes

import (
	"reflect"
	"testing"
)

func TestParseReleaseNotes(t *testing.T) {
	cases := []struct {
		name, input    string
		want           []string
		grouped, valid bool
	}{
		{"empty", "", []string{}, false, true},
		{"null", "null", []string{}, false, true},
		{"legacy", `["旧说明"]`, []string{"旧说明"}, false, true},
		{"categories", `{"notices":["重新登录"],"bug_fixes":["修复"],"features":["功能"],"improvements":["优化"]}`, []string{"功能", "优化", "修复", "重新登录"}, true, true},
		{"missing", `{}`, []string{}, true, true},
		{"nullable", `{"features":null}`, []string{}, true, true},
		{"broken", `broken`, nil, false, false},
		{"unknown", `{"note":[]}`, nil, false, false},
		{"wrong entry", `{"features":[123]}`, nil, false, false},
		{"wrong category", `{"features":"x"}`, nil, false, false},
		{"trailing", `{} {}`, nil, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			notes, grouped, err := ParseReleaseNotes(tc.input)
			if (err == nil) != tc.valid {
				t.Fatalf("error = %v", err)
			}
			if !tc.valid {
				return
			}
			if !reflect.DeepEqual(notes, tc.want) || (grouped != nil) != tc.grouped {
				t.Fatalf("got %#v, %#v", notes, grouped)
			}
			if grouped != nil && (grouped.Features == nil || grouped.Improvements == nil || grouped.BugFixes == nil || grouped.Notices == nil) {
				t.Fatal("categories must serialize as arrays")
			}
		})
	}
}
