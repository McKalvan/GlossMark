// @gloss implements gmi:SurfaceRules scope=file

// Package surface recognises the surfaces the ambiguous-surfaces lab
// modelled (glossmark-examples, lab/ambiguous-surfaces): a page, an admin
// value editor, a CLI, a BFF, a GraphQL gateway and a generated document.
// Each has one marking rule, drawn from the lab's decision rule (ADR
// 0041). gloss check hints when a marker contradicts the rule (GM250 to
// GM254), and gloss suggest and the MCP propose_link default to it.
package surface

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
)

// Kind is the surface a file is.
type Kind string

const (
	None      Kind = ""
	Page      Kind = "page"           // a server-rendered or client page, view or template
	AdminPage Kind = "admin-page"     // a page under admin/, settings/, console/: a value editor
	CLI       Kind = "cli"            // a command-line entry point
	BFF       Kind = "bff"            // a backend-for-frontend or API gateway route
	GraphQL   Kind = "graphql"        // GraphQL resolvers or server
	Schema    Kind = "graphql-schema" // GraphQL SDL
	Document  Kind = "document"       // a renderer of a document that is the product (a PDF invoice)
)

// Rule is the marking rule of one surface, as docs/MARKING.md states it.
type Rule struct {
	Kind    Kind
	Surface string // the surface, as a noun phrase: "a server-rendered page"
	Text    string // the rule, one sentence
	Lab     string // the lab case that settled it
}

var rules = map[Kind]Rule{
	Page:      {Page, "a server-rendered page", "the view that computes the concept implements it (add presents when it also shows it); presents alone is for a view of what an engine elsewhere computes; a page with tests of its own is a page concept", "server-page"},
	AdminPage: {AdminPage, "an admin page", "a page that edits a constraint's value configures it (and its manifest declares configures:); as a page concept it declares its own criticality", "admin-limit"},
	CLI:       {CLI, "a CLI command", "the command that does the work implements it; the group hosts a system-type, and hosts is never the only marker on a process", "cli"},
	BFF:       {BFF, "a BFF route", "the route presents the backend process it reshapes; the rule it re-applies is implemented on the declaration that applies it; the BFF's own concept presents, never realizes, the backend's process", "bff"},
	GraphQL:   {GraphQL, "a GraphQL resolver", "the resolver presents the process it delegates; the server hosts a system-type; a schema type represents the entity, and the holder's manifest says who owns it", "graphql"},
	Schema:    {Schema, "a GraphQL schema", "a schema type represents the entity; the holder's manifest (holds:) says who owns it", "graphql"},
	Document:  {Document, "a document renderer", "a renderer whose output is the product produces the entity: not presents, not represents, not implements", "pdf-invoice"},
}

// RuleFor returns the rule of a surface kind; ok is false for None.
func RuleFor(k Kind) (Rule, bool) {
	r, ok := rules[k]
	return r, ok
}

// Rules lists the rules in docs order.
func Rules() []Rule {
	out := []Rule{rules[Page], rules[AdminPage], rules[CLI], rules[BFF], rules[GraphQL], rules[Document]}
	return out
}

var (
	// Library signals are read on import lines only, so a file that merely
	// mentions a library (this one) is not the surface.
	pdfLib    = regexp.MustCompile(`(?im)^\s*(import|from|require|use|using|require_relative)\b[^\n]*\b(reportlab|fpdf|weasyprint|pdfkit|jspdf|pdfmake|pdf-lib|gofpdf|go-pdf|prawn|wicked_pdf|itextpdf|pdfbox|wkhtmltopdf|pdfgen|pdfkit)\b|\bnew (PDFDocument|jsPDF)\(|\bpage\.pdf\(`)
	cliLib    = regexp.MustCompile(`@click\.(command|group)\b|\bclick\.(Group|Command)\(|argparse\.ArgumentParser|typer\.Typer\(|@app\.command\b|cobra\.Command\{|urfave/cli|["']commander["']|["']yargs["']|#\[derive\([^)]*\bParser\b|\bclap::|picocli|< Thor\b|OptionParser\.new`)
	cliShell  = regexp.MustCompile(`@click\.group\b|\bclick\.Group\(|add_subparsers\(|\.add_command\(|\.AddCommand\(|new Command\(\)|@app\.callback\b|rootCmd`)
	gqlLib    = regexp.MustCompile(`(?im)^\s*(import|from|require|use|using)\b[^\n]*(["'](graphql|@apollo/|apollo-server|@graphql|graphql-yoga|mercurius|type-graphql|@nestjs/graphql)|\b(graphene|strawberry|ariadne)\b|99designs/gqlgen|graphql-java|graphql\.schema)`)
	gqlServer = regexp.MustCompile(`new ApolloServer\(|startStandaloneServer\(|graphqlHTTP\(|createYoga\(|new GraphQLSchema\(|GraphQLView\.as_view|graphene\.Schema\(|strawberry\.Schema\(|handler\.NewDefaultServer\(`)
	pageCode  = regexp.MustCompile(`\brender_template\(|\brender\(request\b|TemplateResponse\(|\bController\b.*<\s*ApplicationController|@Controller\(|\brender\s+:|\.render\(["']`)
	adminDir  = regexp.MustCompile(`(?i)^(admin|admins|settings|console|backoffice|back-office|manage)$`)
	bffDir    = regexp.MustCompile(`(?i)^(bff|[\w-]*-bff|bff-[\w-]*|gateway|api-gateway|[\w-]*-gateway)$`)
	pageDir   = regexp.MustCompile(`(?i)^(pages|views|templates|controllers|screens)$`)
	pageBase  = regexp.MustCompile(`(?i)^(views|page|layout|_app|app|_document)$|(Page|Screen|View)$`)
	pageExt   = map[string]bool{".html": true, ".htm": true, ".erb": true, ".jinja": true, ".jinja2": true, ".j2": true, ".hbs": true, ".ejs": true, ".njk": true, ".vue": true, ".svelte": true, ".tsx": true, ".jsx": true}
	cliBase   = regexp.MustCompile(`(?i)^(cli|__main__|main|cmd)$`)
	// webExt are the languages a page directory (views/, controllers/) is
	// a page surface in; a Go or Rust package named views is not.
	webExt = map[string]bool{".py": true, ".rb": true, ".php": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".vue": true, ".svelte": true, ".html": true, ".htm": true, ".erb": true, ".jinja": true, ".jinja2": true, ".j2": true, ".hbs": true, ".ejs": true, ".njk": true, ".twig": true, ".cshtml": true, ".razor": true, ".ex": true, ".heex": true, ".eex": true}
)

// Detect reports the surface a file is, from its path and, when given, its
// lines. Signals come in this order: GraphQL SDL by extension; a document
// renderer by the PDF library it imports or "pdf" in its name; a CLI by its
// command framework or a cli/ path; GraphQL by its imports or a resolvers
// file; a BFF by a bff/ or gateway/ directory; a page by a pages/, views/,
// templates/ or controllers/ directory, a template extension, a view
// module or a page component. A page under admin/, settings/ or console/
// is an admin page.
func Detect(path string, lines []string) Kind {
	p := filepath.ToSlash(path)
	ext := strings.ToLower(filepath.Ext(p))
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	dirs := strings.Split(filepath.ToSlash(filepath.Dir(p)), "/")
	text := strings.Join(lines, "\n")
	switch {
	case ext == ".graphql" || ext == ".gql":
		return Schema
	case pdfLib.MatchString(text) || strings.Contains(strings.ToLower(base), "pdf"):
		return Document
	case cliLib.MatchString(text) || hasDir(dirs, "cli") || (cliBase.MatchString(base) && (ext == ".py" || ext == ".ts" || ext == ".js" || ext == ".rb")):
		return CLI
	case gqlLib.MatchString(text) || strings.Contains(strings.ToLower(base), "resolver") || hasDir(dirs, "graphql") || hasDir(dirs, "resolvers"):
		return GraphQL
	case anyDir(dirs, bffDir):
		return BFF
	}
	page := false
	switch {
	case anyDir(dirs, pageDir) && webExt[ext] && !(hasDir(dirs, "api") && hasDir(dirs, "pages")):
		page = true
	case pageExt[ext] && pageBase.MatchString(base):
		page = true
	case ext != ".tsx" && ext != ".jsx" && pageExt[ext]:
		page = true
	case pageCode.MatchString(text):
		page = true
	}
	if !page {
		return None
	}
	if anyDir(dirs, adminDir) {
		return AdminPage
	}
	return Page
}

// Shell reports a file that is the shell of its surface rather than one of
// its commands, routes or pages: a CLI group, a GraphQL server, an app
// frame or layout. hosts belongs there, targeting a system-type.
func Shell(k Kind, path string, lines []string) bool {
	text := strings.Join(lines, "\n")
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch k {
	case CLI:
		return cliShell.MatchString(text)
	case GraphQL:
		return gqlServer.MatchString(text)
	case Page, AdminPage:
		return base == "layout" || base == "_app" || base == "App" || base == "_document"
	}
	return false
}

func hasDir(dirs []string, name string) bool {
	for _, d := range dirs {
		if strings.EqualFold(d, name) {
			return true
		}
	}
	return false
}

func anyDir(dirs []string, re *regexp.Regexp) bool {
	for _, d := range dirs {
		if re.MatchString(d) {
			return true
		}
	}
	return false
}

// Link is one marker, written or proposed, on a surface file.
type Link struct {
	Path     string
	Lines    []string // the file's lines, for the shell signals (optional)
	Relation string
	Target   *graph.Node // the resolved concept
	// Artifact is the anchored artifact the marker binds to, when it exists:
	// its other markers say whether the declaration also implements or
	// presents something.
	Artifact *graph.Node
}

// Hint is a rule the link contradicts and the rewrite it suggests.
type Hint struct {
	Code     string
	Relation string // the relation the rule asks for
	Msg      string
}

// Advise reports the surface rule a link contradicts, or nil. Kind is the
// surface of the link's file; the caller detects it once per file.
func Advise(g *graph.Graph, ws *load.Workspace, k Kind, l Link) *Hint {
	t := l.Target
	if k == None || t == nil || t.NodeType != model.NodeConcept || t.Kind == "" || t.Imported {
		return nil
	}
	rule := rules[k]
	tail := fmt.Sprintf(" [surface rule: %s]", rule.Lab)
	switch k {
	case Document:
		// GM254: the output is the product.
		switch l.Relation {
		case "presents", "represents", "implements":
			ent := producedEntity(g, t)
			msg := fmt.Sprintf("%s %s in %s: a renderer whose output is the product produces the entity, not %s; write `produces %s` and declare produces: [%s] in the manifest", l.Relation, t.CURIE, rule.Surface, verb(l.Relation), ent, ent)
			if l.Relation == "implements" && t.Tier == model.TierImpl {
				msg = fmt.Sprintf("implements %s in %s: a concept realized from a renderer takes the engine's tests and moves the process it realizes out of the engine's box; the renderer produces the entity instead: write `produces %s` and declare produces: [%s] in the manifest", t.CURIE, rule.Surface, ent, ent)
			}
			return &Hint{"GM254", "produces", msg + tail}
		}
		return nil
	case Schema:
		return nil
	}
	shell := Shell(k, l.Path, l.Lines)
	// GM253: a page that edits a constraint configures it.
	if k == Page || k == AdminPage {
		switch {
		case l.Relation == "presents" && t.Kind == "constraint":
			return &Hint{"GM253", "configures", fmt.Sprintf("presents %s in %s: a page that edits a constraint's value configures it: write `configures %s` (and configures: [%s] in the manifest); only a page that shows the limit without changing it stays presents", t.CURIE, rule.Surface, t.CURIE, t.CURIE) + tail}
		case l.Relation == "implements" && t.Kind == "constraint":
			if by := implementers(g, ws, t, l.Path, true); len(by) > 0 {
				return &Hint{"GM253", "configures", fmt.Sprintf("implements %s in %s: %s already enforces it, and implements here draws the constraint inside this boundary; a page that edits the value configures it: write `configures %s` (and configures: [%s] in the manifest)", t.CURIE, rule.Surface, by[0], t.CURIE, t.CURIE) + tail}
			}
		case l.Relation == "implements" && t.Tier == model.TierImpl:
			for _, o := range g.Objects(t.ID, "presents") {
				if on := g.Node(o); on != nil && on.Kind == "constraint" && t.CriticalityFrom != "declared" {
					return &Hint{"GM253", "", fmt.Sprintf("implements %s in %s: a page concept that presents constraint %s with no criticality of its own rates the page none; a page that edits the value declares criticality: on its concept (the constraint's level), or configures %s directly", t.CURIE, rule.Surface, on.CURIE, on.CURIE) + tail}
				}
			}
		}
	}
	// GM251: hosts is for the shell, and targets a system-type.
	if l.Relation == "hosts" && t.Kind != "system-type" && len(g.Objects(t.ID, "presents"))+len(g.Objects(t.ID, "hosts")) == 0 {
		part, frame := part(k)
		if shell {
			return &Hint{"GM251", "", fmt.Sprintf("hosts %s on %s: %s is a %s, and %s hosts a system-type; target the system-type it is the shell of (or leave it unmarked), and write `implements %s` on the %s that does the work", t.CURIE, frame, t.CURIE, t.Kind, frame, t.CURIE, part) + tail}
		}
		return &Hint{"GM251", "implements", fmt.Sprintf("hosts %s on %s: write `implements %s` if this code does the work, `presents %s` if it shows what an engine elsewhere does; hosts belongs on %s and targets a system-type (hosts alone hides a missing mechanism)", t.CURIE, rule.Surface, t.CURIE, t.CURIE, frame) + tail}
	}
	// GM250: a surface with no engine behind it is the engine. An entity
	// has no mechanism, and a concept something represents is satisfied
	// by its data shape, as drift's presented-only reads it.
	if l.Relation == "presents" && !shell && mechanismLess(g, t) && len(implementers(g, ws, t, l.Path, false)) == 0 && !declImplements(g, l.Artifact) {
		return &Hint{"GM250", "implements", fmt.Sprintf("presents %s in %s, and nothing implements it; the surface that computes the concept is its mechanism: write `implements %s` on this declaration (keep presents when it also shows it), or mark the engine that does", t.CURIE, rule.Surface, t.CURIE) + tail}
	}
	if l.Relation == "implements" && t.Tier == model.TierImpl && !shell {
		for _, o := range g.Objects(t.ID, "presents") {
			if on := g.Node(o); on != nil && on.NodeType == model.NodeConcept && !on.Imported && mechanismLess(g, on) && len(implementers(g, ws, on, l.Path, false)) == 0 && !declImplementsOther(g, l.Artifact, t.ID) {
				return &Hint{"GM250", "implements", fmt.Sprintf("implements %s, a page concept that presents %s, in %s, and nothing implements %s; the surface that computes the concept is its mechanism: add `implements %s` on this declaration, or mark the engine that does", t.CURIE, on.CURIE, rule.Surface, on.CURIE, on.CURIE) + tail}
			}
		}
	}
	// GM252: an API surface that only re-implements the backend's process.
	if l.Relation == "implements" && (k == BFF || k == GraphQL) && !filePresents(g, l.Path) {
		part, _ := part(k)
		core := t
		if t.Tier == model.TierImpl {
			core = nil
			for _, o := range g.Objects(t.ID, "realizes") {
				if on := g.Node(o); on != nil && on.NodeType == model.NodeConcept {
					core = on
					break
				}
			}
		}
		if core != nil && core.Kind != "constraint" {
			if by := implementers(g, ws, core, l.Path, true); len(by) > 0 {
				if core == t {
					return &Hint{"GM252", "presents", fmt.Sprintf("implements %s in %s: %s already implements it, so this %s re-implements the backend's process and draws it into this boundary; a %s presents the process it reshapes: write `presents %s` here, and `implements` only on the declaration that re-applies a rule of its own", t.CURIE, rule.Surface, by[0], part, part, t.CURIE) + tail}
				}
				return &Hint{"GM252", "presents", fmt.Sprintf("implements %s in %s: it realizes %s, which %s already implements, so the process gets two mechanisms and moves to the root box; a surface's own concept presents the backend's process (presents: %s on %s) and realizes only the rule it re-applies, or the %s writes `presents %s` directly", t.CURIE, rule.Surface, core.CURIE, by[0], core.CURIE, t.CURIE, part, core.CURIE) + tail}
			}
		}
	}
	return nil
}

// Default returns the relation the surface rule asks for on a link from
// path to target, or "" when the rule has no preference: produces on a
// document renderer, configures on an admin page editing a constraint,
// presents on a page, CLI, BFF or GraphQL surface for a concept code under
// another manifest already implements. For a concept nothing implements
// yet the rule has no preference: only a reader can tell a view that
// computes from one that shows (the lab's delete test).
func Default(g *graph.Graph, ws *load.Workspace, k Kind, path string, target *graph.Node) string {
	if target == nil || target.NodeType != model.NodeConcept {
		return ""
	}
	switch k {
	case Document:
		if target.Kind == "entity" {
			return "produces"
		}
	case AdminPage:
		if target.Kind == "constraint" {
			return "configures"
		}
		fallthrough
	case Page, CLI, BFF, GraphQL:
		if target.Tier != model.TierCore {
			return ""
		}
		switch target.Kind {
		case "process", "capability", "constraint", "measure":
			if len(implementers(g, ws, target, path, true)) > 0 {
				return "presents"
			}
		}
	case Schema:
		if target.Kind == "entity" {
			return "represents"
		}
	}
	return ""
}

// PrefersCore reports a surface whose markers name the core concept it
// shows or produces rather than the implementation concept of the engine
// behind it: the page, the route, the resolver, the renderer.
func PrefersCore(k Kind) bool {
	switch k {
	case Page, AdminPage, BFF, GraphQL, Document, Schema:
		return true
	}
	return false
}

// implementers lists the paths of artifacts that implement concept t,
// directly or through an implementation concept that realizes it, other
// than the file at path. With elsewhere, only artifacts under a different
// manifest count.
func implementers(g *graph.Graph, ws *load.Workspace, t *graph.Node, path string, elsewhere bool) []string {
	self := ws.ManifestFor(path)
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		for _, e := range g.In(id, "implements") {
			a := g.Node(e.S)
			if a == nil || a.NodeType != model.NodeArtifact || a.Path == path || seen[a.Path] {
				continue
			}
			if elsewhere && ws.ManifestFor(a.Path) == self {
				continue
			}
			seen[a.Path] = true
			out = append(out, a.Path)
		}
	}
	add(t.ID)
	for _, e := range g.In(t.ID, "realizes") {
		if s := g.Node(e.S); s != nil && s.NodeType == model.NodeConcept {
			add(s.ID)
		}
	}
	sort.Strings(out)
	return out
}

// Implemented reports whether code under another manifest than path's
// implements t, directly or through an implementation concept.
func Implemented(g *graph.Graph, ws *load.Workspace, t *graph.Node, path string) bool {
	return len(implementers(g, ws, t, path, true)) > 0
}

// mechanismLess reports a concept of a kind that has a mechanism (not an
// entity or a resource) which no artifact represents, directly or through
// a concept that realizes it.
func mechanismLess(g *graph.Graph, t *graph.Node) bool {
	if t.Kind == "entity" || t.Kind == "resource" {
		return false
	}
	for _, x := range append([]string{t.ID}, g.Subjects(t.ID, "realizes")...) {
		for _, e := range g.In(x, "represents") {
			if a := g.Node(e.S); a != nil && a.NodeType == model.NodeArtifact {
				return false
			}
		}
	}
	return true
}

// declImplements reports an artifact with an implements marker of its own.
func declImplements(g *graph.Graph, a *graph.Node) bool {
	return a != nil && len(g.Out(a.ID, "implements")) > 0
}

// declImplementsOther reports an artifact that implements something other
// than concept id.
func declImplementsOther(g *graph.Graph, a *graph.Node, id string) bool {
	if a == nil {
		return false
	}
	for _, e := range g.Out(a.ID, "implements") {
		if e.O != id {
			return true
		}
	}
	return false
}

// filePresents reports a file one of whose markers presents a concept,
// inline or through a page concept.
func filePresents(g *graph.Graph, path string) bool {
	for _, a := range g.NodesOf(model.NodeArtifact) {
		if a.Path != path {
			continue
		}
		if len(g.Out(a.ID, "presents")) > 0 {
			return true
		}
		for _, o := range g.Objects(a.ID, "implements") {
			if len(g.Objects(o, "presents")) > 0 {
				return true
			}
		}
	}
	return false
}

// producedEntity names the entity a document renderer produces: the
// target when it is one, else the entity whose name shares its stem with
// the target's (hp:Invoicing, hpx:InvoiceDocument: hp:Invoice), else a
// placeholder.
func producedEntity(g *graph.Graph, t *graph.Node) string {
	if t.Kind == "entity" {
		return t.CURIE
	}
	name := strings.ToLower(local(t.CURIE))
	best, bestLen := "", 0
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.Kind != "entity" || n.Imported {
			continue
		}
		ln := strings.ToLower(local(n.CURIE))
		l := commonPrefix(name, ln)
		if l >= 5 && l >= len(ln)-2 && (l > bestLen || l == bestLen && n.CURIE < best) {
			best, bestLen = n.CURIE, l
		}
	}
	if best == "" {
		return "<the entity the document is>"
	}
	return best
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func local(curie string) string {
	if i := strings.LastIndex(curie, ":"); i >= 0 {
		return curie[i+1:]
	}
	return curie
}

func verb(rel string) string {
	switch rel {
	case "presents":
		return "presents (which rates the file none)"
	case "represents":
		return "represents (which moves the entity to the root box)"
	}
	return "implements"
}

// part names a surface's unit of work and its shell.
func part(k Kind) (unit, shell string) {
	switch k {
	case CLI:
		return "command", "the command group"
	case GraphQL:
		return "resolver", "the GraphQL server"
	case BFF:
		return "route", "the router"
	}
	return "view", "the app frame"
}
