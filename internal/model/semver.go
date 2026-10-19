// @gloss implements gmi:ImportLock scope=file

package model

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver is a parsed MAJOR.MINOR.PATCH version (pre-release and build
// metadata are accepted and ignored for ordering).
type Semver struct {
	Major, Minor, Patch int
	Pre                 string
}

// ParseSemver parses "1.2.3", "v1.2.3", "1.2" (patch 0) or "1" (minor and patch 0).
func ParseSemver(s string) (Semver, error) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "v"))
	if s == "" {
		return Semver{}, fmt.Errorf("empty version")
	}
	var v Semver
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		v.Pre = s[i:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return Semver{}, fmt.Errorf("invalid version %q", s)
	}
	nums := []*int{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Semver{}, fmt.Errorf("invalid version %q", s)
		}
		*nums[i] = n
	}
	return v, nil
}

func (v Semver) String() string {
	return fmt.Sprintf("%d.%d.%d%s", v.Major, v.Minor, v.Patch, v.Pre)
}

// Compare returns -1, 0 or 1.
func (v Semver) Compare(o Semver) int {
	for _, d := range []int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
	}
	return 0
}

// Constraint is a version range: a conjunction of comparators.
type Constraint struct {
	raw   string
	terms []term
}

type term struct {
	op string // "=", ">", ">=", "<", "<="
	v  Semver
}

// ParseConstraint accepts npm/Cargo-style ranges:
//
//	1.2.3        exactly
//	^1.2  ^1.2.3 same major (same minor when major is 0)
//	~1.2.3       same major.minor
//	1.x  1.2.x   wildcard
//	>=1.2 <2.0   space-separated conjunction
//	*            anything
func ParseConstraint(s string) (Constraint, error) {
	c := Constraint{raw: strings.TrimSpace(s)}
	if c.raw == "" || c.raw == "*" {
		return c, nil
	}
	for _, f := range strings.Fields(c.raw) {
		ts, err := parseRangeTerm(f)
		if err != nil {
			return Constraint{}, err
		}
		c.terms = append(c.terms, ts...)
	}
	return c, nil
}

func parseRangeTerm(f string) ([]term, error) {
	switch {
	case strings.HasPrefix(f, "^"):
		v, err := ParseSemver(f[1:])
		if err != nil {
			return nil, err
		}
		upper := Semver{Major: v.Major + 1}
		if v.Major == 0 {
			upper = Semver{Minor: v.Minor + 1}
		}
		return []term{{">=", v}, {"<", upper}}, nil
	case strings.HasPrefix(f, "~"):
		v, err := ParseSemver(f[1:])
		if err != nil {
			return nil, err
		}
		return []term{{">=", v}, {"<", Semver{Major: v.Major, Minor: v.Minor + 1}}}, nil
	case strings.HasPrefix(f, ">="), strings.HasPrefix(f, "<="):
		v, err := ParseSemver(f[2:])
		if err != nil {
			return nil, err
		}
		return []term{{f[:2], v}}, nil
	case strings.HasPrefix(f, ">"), strings.HasPrefix(f, "<"):
		v, err := ParseSemver(f[1:])
		if err != nil {
			return nil, err
		}
		return []term{{f[:1], v}}, nil
	case strings.HasPrefix(f, "="):
		v, err := ParseSemver(f[1:])
		if err != nil {
			return nil, err
		}
		return []term{{"=", v}}, nil
	case strings.Contains(f, "x") || strings.Contains(f, "*"):
		parts := strings.Split(strings.TrimPrefix(f, "v"), ".")
		var nums []int
		for _, p := range parts {
			if p == "x" || p == "*" || p == "X" {
				break
			}
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil, fmt.Errorf("invalid range %q", f)
			}
			nums = append(nums, n)
		}
		switch len(nums) {
		case 0:
			return nil, nil
		case 1:
			return []term{{">=", Semver{Major: nums[0]}}, {"<", Semver{Major: nums[0] + 1}}}, nil
		default:
			return []term{{">=", Semver{Major: nums[0], Minor: nums[1]}}, {"<", Semver{Major: nums[0], Minor: nums[1] + 1}}}, nil
		}
	default:
		v, err := ParseSemver(f)
		if err != nil {
			return nil, err
		}
		// A bare partial version ("1.2") means that minor; a full one means exactly.
		dots := strings.Count(strings.TrimPrefix(f, "v"), ".")
		if dots == 2 {
			return []term{{"=", v}}, nil
		}
		if dots == 1 {
			return []term{{">=", v}, {"<", Semver{Major: v.Major, Minor: v.Minor + 1}}}, nil
		}
		return []term{{">=", v}, {"<", Semver{Major: v.Major + 1}}}, nil
	}
}

// Satisfies reports whether v is inside the range.
func (c Constraint) Satisfies(v Semver) bool {
	for _, t := range c.terms {
		cmp := v.Compare(t.v)
		switch t.op {
		case "=":
			if cmp != 0 {
				return false
			}
		case ">":
			if cmp <= 0 {
				return false
			}
		case ">=":
			if cmp < 0 {
				return false
			}
		case "<":
			if cmp >= 0 {
				return false
			}
		case "<=":
			if cmp > 0 {
				return false
			}
		}
	}
	return true
}

// String returns the range as written.
func (c Constraint) String() string { return c.raw }

// IsAny reports whether the constraint accepts every version.
func (c Constraint) IsAny() bool { return len(c.terms) == 0 }
