package marker

import (
	"path/filepath"
	"strings"
)

// CommentStyle lists the line-comment tokens and block-comment pairs for a file type.
type CommentStyle struct {
	Line  []string
	Block [][2]string
}

var (
	hash  = CommentStyle{Line: []string{"#"}}
	slash = CommentStyle{Line: []string{"//"}, Block: [][2]string{{"/*", "*/"}}}
	dash  = CommentStyle{Line: []string{"--"}, Block: [][2]string{{"/*", "*/"}}}
	xml   = CommentStyle{Block: [][2]string{{"<!--", "-->"}}}
	semi  = CommentStyle{Line: []string{";"}}
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
	".md": xml, ".mdx": xml, ".html": xml, ".htm": xml, ".xml": xml, ".svg": xml, ".vue": xml, ".xsl": xml, ".xhtml": xml,
	".lisp": semi, ".clj": semi, ".cljs": semi, ".edn": semi, ".el": semi, ".scm": semi, ".rkt": semi, ".asm": semi, ".s": semi,
	".gloss": sidecar,
}

// StyleFor returns the comment style for a path, honouring overrides
// (extension → tokens) from glossmark.yaml. ok is false for unknown or
// comment-less types (JSON needs a sidecar).
func StyleFor(path string, overrides map[string][]string) (CommentStyle, bool) {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	if overrides != nil {
		if toks, ok := overrides[ext]; ok {
			return styleFromTokens(toks), true
		}
		if toks, ok := overrides[base]; ok {
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
