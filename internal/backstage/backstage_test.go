// @gloss verifies gmi:BackstageExport scope=file

package backstage

import "testing"

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"claimb:assessment", "claimb-assessment"},
		{"arch:lib-projection", "arch-lib-projection"},
		{"bw:HireHQ_Invoicing", "bw-hirehq-invoicing"},
		{"a::b", "a-b"},
		{":x:", "x"},
	} {
		if got := Slug(tc.in); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
