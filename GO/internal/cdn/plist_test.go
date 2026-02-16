package cdn

import (
	"testing"
)

func TestParsePlistPackages(t *testing.T) {
	xmlStr := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<array>
  <dict>
    <key>Location</key>
    <string>https://officecdnmac.microsoft.com/pr/xxx/Microsoft_Word_16.93_Updater.pkg</string>
    <key>Update Version</key>
    <string>16.93.25011212</string>
    <key>File Size</key>
    <integer>1048576000</integer>
  </dict>
  <dict>
    <key>Location</key>
    <string>https://officecdnmac.microsoft.com/pr/xxx/Microsoft_Word_16.92_to_16.93_Delta.pkg</string>
    <key>BinaryUpdaterLocation</key>
    <string>https://officecdnmac.microsoft.com/pr/xxx/Microsoft_Word_Binary.pkg</string>
    <key>Update Version</key>
    <string>16.93.25011212</string>
  </dict>
</array>
</plist>`

	pkgs, err := ParsePlistPackages(xmlStr)
	if err != nil {
		t.Fatalf("ParsePlistPackages failed: %v", err)
	}

	uris := pkgs.AllURIs()
	if len(uris) != 3 {
		t.Errorf("expected 3 unique URIs, got %d: %v", len(uris), uris)
	}

	if len(pkgs.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(pkgs.Versions))
	}
	if pkgs.Versions[0] != "16.93.25011212" {
		t.Errorf("expected version 16.93.25011212, got %s", pkgs.Versions[0])
	}
}

func TestParsePlistVersion(t *testing.T) {
	xmlStr := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>Update Version</key>
  <string>16.93.25011212</string>
  <key>Date</key>
  <string>2025-01-12</string>
  <key>Type</key>
  <string>Production</string>
</dict>
</plist>`

	version := ParsePlistVersion(xmlStr)
	if version != "16.93.25011212" {
		t.Errorf("expected 16.93.25011212, got %s", version)
	}
}

func TestParsePlistVersionUnknown(t *testing.T) {
	version := ParsePlistVersion("<plist><dict></dict></plist>")
	if version != "unknown" {
		t.Errorf("expected unknown, got %s", version)
	}
}

func TestParsePlistStringArray(t *testing.T) {
	xmlStr := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<array>
  <string>16.90.24121212</string>
  <string>16.91.25010101</string>
  <string>16.92.25010212</string>
</array>
</plist>`

	versions := ParsePlistStringArray(xmlStr)
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	if versions[0] != "16.90.24121212" {
		t.Errorf("expected 16.90.24121212, got %s", versions[0])
	}
	if versions[2] != "16.92.25010212" {
		t.Errorf("expected 16.92.25010212, got %s", versions[2])
	}
}

func TestParsePlistBooleanValues(t *testing.T) {
	xmlStr := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>IsActive</key>
  <true/>
  <key>IsRetired</key>
  <false/>
</dict>
</plist>`

	version := ParsePlistVersion(xmlStr)
	if version != "unknown" {
		t.Errorf("expected unknown (no Update Version), got %s", version)
	}
}

func TestBuildVersionedURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		version  string
		newExt   string
		expected string
	}{
		{
			name:     "versioned cat",
			uri:      "https://officecdnmac.microsoft.com/pr/xxx/MacAutoupdate/0409MSWD2019.cat",
			version:  "16.93.25011212",
			newExt:   "",
			expected: "https://officecdnmac.microsoft.com/pr/xxx/MacAutoupdate/0409MSWD2019_16.93.25011212.cat",
		},
		{
			name:     "versioned xml",
			uri:      "https://officecdnmac.microsoft.com/pr/xxx/MacAutoupdate/0409MSWD2019.cat",
			version:  "16.93.25011212",
			newExt:   ".xml",
			expected: "https://officecdnmac.microsoft.com/pr/xxx/MacAutoupdate/0409MSWD2019_16.93.25011212.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildVersionedURI(tt.uri, tt.version, tt.newExt)
			if got != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestParsePlistEmptyInput(t *testing.T) {
	versions := ParsePlistStringArray("")
	if versions != nil {
		t.Errorf("expected nil for empty input, got %v", versions)
	}

	version := ParsePlistVersion("")
	if version != "unknown" {
		t.Errorf("expected unknown for empty input, got %s", version)
	}
}
