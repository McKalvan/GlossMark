package version

import "testing"

func TestIsDevBuild(t *testing.T) {
	for _, tc := range []struct {
		v   string
		dev bool
	}{
		{"", true},
		{"(devel)", true},
		// No earlier tag: vX.0.0-yyyymmddhhmmss-abcdef123456.
		{"v0.0.0-20261020035758-2007e0df1a9e", true},
		// After a pre-release tag: vX.Y.Z-pre.0.yyyymmddhhmmss-abcdef123456.
		{"v0.2.0-rc.1.0.20261020035758-2007e0df1a9e", true},
		// After a release tag: vX.Y.(Z+1)-0.yyyymmddhhmmss-abcdef123456.
		{"v0.1.1-0.20261020035758-2007e0df1a9e", true},
		{"v0.1.1-0.20261020035758-2007e0df1a9e+dirty", true},
		{"v2.0.1-0.20261020035758-2007e0df1a9e+incompatible", true},
		{"v0.1.0+dirty", true},
		{"v0.1.0", false},
		{"v0.2.0-rc.1", false},
		{"v1.2.3-beta.20261020", false},
	} {
		if got := isDevBuild(tc.v); got != tc.dev {
			t.Errorf("isDevBuild(%q) = %v, want %v", tc.v, got, tc.dev)
		}
	}
}
