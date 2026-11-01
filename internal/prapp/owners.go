// @gloss implements gmi:PullRequestApp scope=file

package prapp

import (
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/gate"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// Reviewers are the GitHub accounts to ask for a review: logins and team
// slugs (the org is the repository's), and the owner: names they stand for.
type Reviewers struct {
	Users  []string
	Teams  []string
	Owners []string // the owner: values the HARD changes belong to, for the summary
	// Unmapped are owners with no app.owners entry and no CODEOWNERS match.
	Unmapped []string
}

// OwnersOf lists the owner: values behind the HARD changes: the owner of
// the namespace of each subject and object, and the owner of the boundary
// whose manifest encloses the change's path.
func OwnersOf(ws *load.Workspace, im *gate.Impact) []string {
	byBoundary := map[string]string{}
	for _, m := range ws.Manifests {
		byBoundary[m.Boundary] = m.Owner
	}
	set := map[string]bool{}
	add := func(o string) {
		if o != "" {
			set[o] = true
		}
	}
	for _, ch := range im.Changes {
		if ch.Gate != gate.HARD {
			continue
		}
		for _, curie := range strings.Split(ch.Subject+", "+ch.Object, ", ") {
			prefix, _, ok := strings.Cut(strings.TrimSpace(curie), ":")
			if !ok {
				continue
			}
			if ns := ws.Config.Owns[prefix]; ns != nil {
				add(ns.Owner)
			}
			add(byBoundary[strings.TrimSpace(curie)])
		}
		add(byBoundary[ch.Boundary])
	}
	out := make([]string, 0, len(set))
	for o := range set {
		out = append(out, o)
	}
	sort.Strings(out)
	return out
}

// MapOwners turns owner: values into reviewers through app.owners in
// glossmark.yaml. An entry is a login (alice, @alice) or a team (@org/slug;
// the org is dropped, the request goes to the repository's). Owners
// without an entry fall back to CODEOWNERS for the HARD changes' paths.
func MapOwners(cfg *model.AppConfig, owners []string, im *gate.Impact, codeowners string) Reviewers {
	var rv Reviewers
	rv.Owners = owners
	seen := map[string]bool{}
	take := func(acct string) {
		name := strings.TrimPrefix(strings.TrimSpace(acct), "@")
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		if _, slug, ok := strings.Cut(name, "/"); ok {
			rv.Teams = append(rv.Teams, slug)
		} else {
			rv.Users = append(rv.Users, name)
		}
	}
	for _, o := range owners {
		var mapped model.StringList
		if cfg != nil {
			mapped = cfg.Owners[o]
		}
		if len(mapped) == 0 {
			rv.Unmapped = append(rv.Unmapped, o)
			continue
		}
		for _, a := range mapped {
			take(a)
		}
	}
	if len(rv.Unmapped) > 0 || len(owners) == 0 {
		rules := ParseCodeowners(codeowners)
		if len(rules) > 0 {
			paths := map[string]bool{}
			for _, ch := range im.Changes {
				if ch.Gate == gate.HARD && ch.Path != "" {
					paths[ch.Path] = true
				}
			}
			for p := range paths {
				for _, a := range CodeownersFor(rules, p) {
					take(a)
				}
			}
		}
	}
	sort.Strings(rv.Users)
	sort.Strings(rv.Teams)
	return rv
}

// CodeownersRule is one CODEOWNERS line.
type CodeownersRule struct {
	Pattern string
	Owners  []string
}

// CodeownersPaths are where GitHub looks for the file, in order.
var CodeownersPaths = []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

// ParseCodeowners reads a CODEOWNERS file: a pattern, then owners
// (@login, @org/team; an email is kept as written), comments dropped.
func ParseCodeowners(text string) []CodeownersRule {
	var rules []CodeownersRule
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		rules = append(rules, CodeownersRule{Pattern: fields[0], Owners: fields[1:]})
	}
	return rules
}

// CodeownersFor returns the owners of path: the last matching rule wins,
// as in GitHub.
func CodeownersFor(rules []CodeownersRule, path string) []string {
	path = strings.TrimPrefix(path, "./")
	var owners []string
	for _, r := range rules {
		if codeownersMatch(r.Pattern, path) {
			owners = r.Owners
		}
	}
	return owners
}

// codeownersMatch applies CODEOWNERS pattern semantics with the
// repository's glob matcher: a leading / anchors at the root, a trailing /
// means everything beneath, a pattern with no wildcard also matches as a
// directory, and a pattern without a slash matches at any depth.
func codeownersMatch(pattern, path string) bool {
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	if pattern == "" || pattern == "*" || pattern == "**" {
		return true
	}
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	if !anchored && !strings.Contains(strings.TrimSuffix(pattern, "/**"), "/") && !strings.HasSuffix(pattern, "/**") {
		// A bare name: any depth (MatchGlob does this when there is no slash).
		return model.MatchGlob(pattern, path)
	}
	if !anchored && !strings.HasPrefix(pattern, "**") {
		// "docs/*.md" without a leading slash matches anywhere in the tree.
		if model.MatchGlob("**/"+pattern, path) || model.MatchGlob("**/"+pattern+"/**", path) {
			return true
		}
	}
	if model.MatchGlob(pattern, path) {
		return true
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return model.MatchGlob(pattern+"/**", path)
	}
	return false
}
