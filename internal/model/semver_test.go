// @gloss verifies gmi:ImportLock scope=file

package model

import "testing"

func TestConstraints(t *testing.T) {
	cases := []struct {
		rng, v string
		want   bool
	}{
		{"^1.2", "1.2.0", true}, {"^1.2", "1.9.3", true}, {"^1.2", "2.0.0", false}, {"^1.2", "1.1.9", false},
		{"^0.3", "0.3.4", true}, {"^0.3", "0.4.0", false},
		{"~1.2.3", "1.2.9", true}, {"~1.2.3", "1.3.0", false},
		{"1.x", "1.7.0", true}, {"1.x", "2.0.0", false}, {"1.2.x", "1.2.5", true}, {"1.2.x", "1.3.0", false},
		{">=1.2 <2.0", "1.5.0", true}, {">=1.2 <2.0", "2.0.0", false},
		{"1.2.0", "1.2.0", true}, {"1.2.0", "1.2.1", false},
		{"1.2", "1.2.7", true}, {"1.2", "1.3.0", false},
		{"*", "9.9.9", true}, {"", "0.0.1", true},
		{"v1.0.0", "1.0.0", true},
	}
	for _, c := range cases {
		cons, err := ParseConstraint(c.rng)
		if err != nil {
			t.Errorf("%q: %v", c.rng, err)
			continue
		}
		v, err := ParseSemver(c.v)
		if err != nil {
			t.Fatal(err)
		}
		if got := cons.Satisfies(v); got != c.want {
			t.Errorf("%q satisfies %q = %v, want %v", c.rng, c.v, got, c.want)
		}
	}
	for _, bad := range []string{"^a", "1.2.3.4", ">=", "~"} {
		if _, err := ParseConstraint(bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}
