// @gloss implements gmi:MarkerGrammar scope=file

package marker

import (
	"path/filepath"
	"strings"

	"github.com/mckalvan/glossmark/internal/model"
)

// CommentStyle lists the line-comment tokens and block-comment pairs for a file type.
type CommentStyle struct {
	Line  []string
	Block [][2]string
	// Fences is true for Markdown-like files: text inside a fenced code block
	// (``` or ~~~) is quoted and never holds a marker.
	Fences bool
}

var (
	hash  = CommentStyle{Line: []string{"#"}}
	slash = CommentStyle{Line: []string{"//"}, Block: [][2]string{{"/*", "*/"}}}
	dash  = CommentStyle{Line: []string{"--"}, Block: [][2]string{{"/*", "*/"}}}
	xml   = CommentStyle{Block: [][2]string{{"<!--", "-->"}}}
	md    = CommentStyle{Block: [][2]string{{"<!--", "-->"}}, Fences: true}
	semi  = CommentStyle{Line: []string{";"}}
	// slashLine is // without block comments (Prisma has none).
	slashLine = CommentStyle{Line: []string{"//"}}
	// Sidecar files (foo.json.gloss) are plain marker lists; '#' comments are allowed.
	sidecar = CommentStyle{Line: []string{"#", ""}}
)

// byExt is the builtin comment table, keyed by lower-case extension (with dot)
// or by exact basename for extension-less files.
var byExt = map[string]CommentStyle{
	".py": hash, ".sh": hash, ".bash": hash, ".zsh": hash, ".yaml": hash, ".yml": hash, ".tf": hash, ".hcl": hash,
	".tfvars": hash, ".toml": hash, ".rb": hash, ".feature": hash, ".ps1": hash, ".r": hash, ".pl": hash, ".pm": hash,
	".mk": hash, ".cfg": hash, ".ini": hash, ".conf": hash, ".properties": hash, ".env": hash, ".nix": hash,
	".ex": hash, ".exs": hash, ".cmake": hash, ".gitignore": hash, ".dockerignore": hash, ".jl": hash, ".nim": hash,
	"Dockerfile": hash, "Makefile": hash, "Jenkinsfile": slash, "Vagrantfile": hash, "Rakefile": hash, "Gemfile": hash, "Procfile": hash,
	".go": slash, ".ts": slash, ".tsx": slash, ".js": slash, ".jsx": slash, ".mjs": slash, ".cjs": slash, ".java": slash,
	".kt": slash, ".kts": slash, ".rs": slash, ".c": slash, ".h": slash, ".cc": slash, ".cpp": slash, ".hpp": slash,
	".cs": slash, ".swift": slash, ".scala": slash, ".php": slash, ".dart": slash, ".proto": slash, ".groovy": slash,
	".gradle": slash, ".m": slash, ".mm": slash, ".zig": slash, ".v": slash, ".sv": slash, ".jsonc": slash, ".json5": slash,
	".sql": dash, ".lua": dash, ".hs": dash, ".elm": dash, ".ada": dash, ".psql": dash, ".ddl": dash,
	".md": md, ".mdx": md, ".html": xml, ".htm": xml, ".xml": xml, ".svg": xml, ".vue": xml, ".xsl": xml, ".xhtml": xml,
	".lisp": semi, ".clj": semi, ".cljs": semi, ".edn": semi, ".el": semi, ".scm": semi, ".rkt": semi, ".asm": semi, ".s": semi,
	".graphql": hash, ".graphqls": hash, ".gql": hash, ".prisma": slashLine,
	".gloss": sidecar,
}

// StyleFor returns the comment style for a path, honouring the
// comment-styles overrides from glossmark.yaml. An override key is an
// extension (".foo"), an exact file name ("crontab"), or a glob over the
// repository-relative path ("**/cron.d/*", any key with '/', '*', '?' or
// '['). The most specific override wins: a file name, then a glob (the
// longest matching pattern), then an extension; overrides win over the
// builtin table. ok is false for unknown or comment-less types (JSON needs a
// sidecar).
func StyleFor(path string, overrides map[string][]string) (CommentStyle, bool) {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	if len(overrides) > 0 {
		if toks, ok := overrides[base]; ok && !isGlobKey(base) {
			return styleFromTokens(toks), true
		}
		best := ""
		for k := range overrides {
			if isGlobKey(k) && model.MatchGlob(k, filepath.ToSlash(path)) && (len(k) > len(best) || len(k) == len(best) && k < best) {
				best = k
			}
		}
		if best != "" {
			return styleFromTokens(overrides[best]), true
		}
		if toks, ok := overrides[ext]; ok && ext != "" {
			return styleFromTokens(toks), true
		}
	}
	if s, ok := byExt[base]; ok {
		return s, true
	}
	if strings.HasPrefix(base, "Dockerfile.") || strings.HasSuffix(base, ".dockerfile") {
		return hash, true
	}
	if s, ok := byExt[ext]; ok {
		return s, true
	}
	return CommentStyle{}, false
}

// isGlobKey reports a comment-styles key that is a path glob rather than an
// extension or a file name.
func isGlobKey(k string) bool {
	return strings.ContainsAny(k, "/*?[")
}

func styleFromTokens(toks []string) CommentStyle {
	var s CommentStyle
	for _, t := range toks {
		if i := strings.Index(t, " "); i > 0 {
			s.Block = append(s.Block, [2]string{t[:i], strings.TrimSpace(t[i+1:])})
		} else {
			s.Line = append(s.Line, t)
		}
	}
	return s
}

// SupportedExtensions returns the builtin table's keys, for documentation.
func SupportedExtensions() []string {
	out := make([]string, 0, len(byExt))
	for k := range byExt {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

// IsSidecar reports whether a path is a JSON sidecar (foo.json.gloss).
func IsSidecar(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".gloss")
}

// SidecarTarget returns the file a sidecar annotates (foo.json for foo.json.gloss).
func SidecarTarget(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path))
}
