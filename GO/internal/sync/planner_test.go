package sync

import (
	"testing"
)

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want int
	}{
		{"no duplicates", []string{"a", "b", "c"}, 3},
		{"with duplicates", []string{"a", "b", "a", "c", "b"}, 3},
		{"empty input", nil, 0},
		{"whitespace trimmed", []string{" a ", "a", " b"}, 2},
		{"blank strings removed", []string{"", "  ", "a"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueStrings(tt.in)
			if len(got) != tt.want {
				t.Errorf("uniqueStrings(%v) returned %d items, want %d", tt.in, len(got), tt.want)
			}
		})
	}
}

func TestUniqueStringsPreservesOrder(t *testing.T) {
	got := uniqueStrings([]string{"c", "a", "b", "a", "c"})
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("uniqueStrings[%d] = %q, want %q", i, got[i], v)
		}
	}
}

func TestDeltaPatternMatching(t *testing.T) {
	tests := []struct {
		uri   string
		match bool
		from  string
		to    string
	}{
		{
			"https://cdn.example.com/pkg_16.90.24121212_to_16.93.25011212_update.pkg",
			true, "16.90.24121212", "16.93.25011212",
		},
		{
			"https://cdn.example.com/pkg_1.2.3_to_4.5.6_update.pkg",
			true, "1.2.3", "4.5.6",
		},
		{
			"https://cdn.example.com/update_full.pkg",
			false, "", "",
		},
		{
			"https://cdn.example.com/no_delta_here.xml",
			false, "", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			matches := deltaPattern.FindStringSubmatch(tt.uri)
			if tt.match {
				if matches == nil {
					t.Fatalf("expected match for %q", tt.uri)
				}
				if matches[1] != tt.from {
					t.Errorf("from = %q, want %q", matches[1], tt.from)
				}
				if matches[2] != tt.to {
					t.Errorf("to = %q, want %q", matches[2], tt.to)
				}
			} else {
				if matches != nil {
					t.Errorf("expected no match for %q, got %v", tt.uri, matches)
				}
			}
		})
	}
}

func TestDeltaFilteringWithBuilds(t *testing.T) {
	builds := []string{"16.90.24121212"}
	buildSet := make(map[string]bool)
	for _, b := range builds {
		buildSet[b] = true
	}

	uris := []string{
		"https://cdn.example.com/pkg_16.90.24121212_to_16.93.25011212_update.pkg",
		"https://cdn.example.com/pkg_16.80.00000000_to_16.93.25011212_update.pkg",
		"https://cdn.example.com/full_update.pkg",
	}

	var filtered []string
	for _, u := range uris {
		matches := deltaPattern.FindStringSubmatch(u)
		if matches == nil {
			filtered = append(filtered, u)
		} else {
			fromVer := matches[1]
			if buildSet[fromVer] {
				filtered = append(filtered, u)
			}
		}
	}

	if len(filtered) != 2 {
		t.Fatalf("filtered count = %d, want 2", len(filtered))
	}
	// First should be the delta with matching from_version
	if filtered[0] != uris[0] {
		t.Errorf("filtered[0] = %q, want %q", filtered[0], uris[0])
	}
	// Second should be the non-delta full package
	if filtered[1] != uris[2] {
		t.Errorf("filtered[1] = %q, want %q", filtered[1], uris[2])
	}
}

func TestNonDeltaAlwaysKept(t *testing.T) {
	buildSet := map[string]bool{} // empty builds

	uris := []string{
		"https://cdn.example.com/full_update.pkg",
		"https://cdn.example.com/another_package.pkg",
	}

	var filtered []string
	for _, u := range uris {
		matches := deltaPattern.FindStringSubmatch(u)
		if matches == nil {
			filtered = append(filtered, u)
		} else {
			fromVer := matches[1]
			if buildSet[fromVer] {
				filtered = append(filtered, u)
			}
		}
	}

	if len(filtered) != len(uris) {
		t.Errorf("filtered = %d, want %d: non-delta packages should always be kept", len(filtered), len(uris))
	}
}
