package cdn

import (
	"testing"
)

func TestTargetAppsCount(t *testing.T) {
	if len(TargetApps) != 20 {
		t.Errorf("TargetApps has %d entries, want 20", len(TargetApps))
	}
}

func TestTargetAppsIDs(t *testing.T) {
	expectedIDs := []string{
		"0409MSau04",
		"0409MSWD2019",
		"0409XCEL2019",
		"0409PPT32019",
		"0409OPIM2019",
		"0409ONMC2019",
		"0409MSWD15",
		"0409XCEL15",
		"0409PPT315",
		"0409OPIM15",
		"0409ONMC15",
		"0409MSFB16",
		"0409IMCP01",
		"0409MSRD10",
		"0409ONDR18",
		"0409WDAV00",
		"0409EDGE01",
		"0409TEAMS10",
		"0409TEAMS21",
		"0409OLIC02",
	}

	if len(TargetApps) != len(expectedIDs) {
		t.Fatalf("TargetApps length = %d, want %d", len(TargetApps), len(expectedIDs))
	}

	for i, want := range expectedIDs {
		if TargetApps[i].AppID != want {
			t.Errorf("TargetApps[%d].AppID = %q, want %q", i, TargetApps[i].AppID, want)
		}
	}
}

func TestChannelPathsGUIDs(t *testing.T) {
	tests := []struct {
		channel string
		want    string
	}{
		{"Production", "/pr/C1297A47-86C4-4C1F-97FA-950631F94777/MacAutoupdate/"},
		{"Preview", "/pr/1ac37578-5a24-40fb-892e-b89d85b6dfaa/MacAutoupdate/"},
		{"Beta", "/pr/4B2D7701-0A4F-49C8-B4CB-0C2D4043F51F/MacAutoupdate/"},
	}
	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			got, ok := channelPaths[tt.channel]
			if !ok {
				t.Fatalf("channelPaths missing key %q", tt.channel)
			}
			if got != tt.want {
				t.Errorf("channelPaths[%q] = %q, want %q", tt.channel, got, tt.want)
			}
		})
	}
}

func TestBuildVersionedURIConstructsCorrectURIs(t *testing.T) {
	tests := []struct {
		name        string
		originalURI string
		version     string
		newExt      string
		want        string
	}{
		{
			name:        "basic with new extension",
			originalURI: "https://officecdnmac.microsoft.com/pr/GUID/MacAutoupdate/0409MSau04.xml",
			version:     "4.73",
			newExt:      ".xml",
			want:        "https://officecdnmac.microsoft.com/pr/GUID/MacAutoupdate/0409MSau04_4.73.xml",
		},
		{
			name:        "keep original extension",
			originalURI: "https://officecdnmac.microsoft.com/pr/GUID/MacAutoupdate/0409MSau04.cat",
			version:     "4.73",
			newExt:      "",
			want:        "https://officecdnmac.microsoft.com/pr/GUID/MacAutoupdate/0409MSau04_4.73.cat",
		},
		{
			name:        "complex version string",
			originalURI: "https://cdn.example.com/path/to/app.xml",
			version:     "16.93.25011212",
			newExt:      ".xml",
			want:        "https://cdn.example.com/path/to/app_16.93.25011212.xml",
		},
		{
			name:        "invalid URI returns empty",
			originalURI: "://bad-uri",
			version:     "1.0",
			newExt:      ".xml",
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildVersionedURI(tt.originalURI, tt.version, tt.newExt)
			if got != tt.want {
				t.Errorf("BuildVersionedURI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChannelBaseURL(t *testing.T) {
	tests := []struct {
		channel string
		want    string
	}{
		{"Production", "https://officecdnmac.microsoft.com/pr/C1297A47-86C4-4C1F-97FA-950631F94777/MacAutoupdate/"},
		{"Preview", "https://officecdnmac.microsoft.com/pr/1ac37578-5a24-40fb-892e-b89d85b6dfaa/MacAutoupdate/"},
		{"Beta", "https://officecdnmac.microsoft.com/pr/4B2D7701-0A4F-49C8-B4CB-0C2D4043F51F/MacAutoupdate/"},
	}
	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			got := ChannelBaseURL(tt.channel)
			if got != tt.want {
				t.Errorf("ChannelBaseURL(%q) = %q, want %q", tt.channel, got, tt.want)
			}
		})
	}
}

func TestChannelBaseURLUnknown(t *testing.T) {
	got := ChannelBaseURL("Unknown")
	// Unknown channel returns cdnBase with empty path
	if got != cdnBase {
		t.Errorf("ChannelBaseURL(Unknown) = %q, want %q", got, cdnBase)
	}
}
