// @gloss verifies gmi:MarkerGrammar scope=file

package marker

import (
	"reflect"
	"testing"
)

// TestStyleForOverrides: comment-styles keys may be an extension, a file
// name or a path glob (issue #156); the most specific one wins, and
// overrides win over the builtin table.
func TestStyleForOverrides(t *testing.T) {
	overrides := map[string][]string{
		".jsp":         {"<%-- --%>"},
		"crontab":      {"#"},
		"**/cron.d/*":  {";"},
		"etc/cron.d/*": {"//"},
		".sh":          {"%"},
	}
	cases := []struct {
		path string
		line []string
		ok   bool
	}{
		{"batch/crontab", []string{"#"}, true},
		{"crontab", []string{"#"}, true},
		{"ops/cron.d/nightly", []string{";"}, true},
		{"etc/cron.d/nightly", []string{"//"}, true}, // the longer glob is more specific
		{"etc/cron.d/crontab", []string{"#"}, true},  // a file name beats a glob
		{"ops/cron.d/run.sh", []string{";"}, true},   // a glob beats an extension
		{"scripts/run.sh", []string{"%"}, true},      // an extension override beats the builtin table
		{"batch/cronjob", nil, false},
		{"web/index.jsp", nil, true},
	}
	for _, c := range cases {
		s, ok := StyleFor(c.path, overrides)
		if ok != c.ok || c.line != nil && !reflect.DeepEqual(s.Line, c.line) {
			t.Errorf("%s: got %v %v, want %v %v", c.path, s.Line, ok, c.line, c.ok)
		}
	}
	if s, _ := StyleFor("web/index.jsp", overrides); len(s.Block) != 1 || s.Block[0] != [2]string{"<%--", "--%>"} {
		t.Errorf("jsp block = %v", s.Block)
	}
	if s, ok := StyleFor("prisma/schema.prisma", nil); !ok || !reflect.DeepEqual(s.Line, []string{"//"}) || len(s.Block) != 0 {
		t.Errorf("prisma = %+v %v, want // line comments", s, ok)
	}
}
