// @gloss implements gmi:Bootstrapper scope=file

// Package bootstrap drafts a first ontology for a repository that has no
// GlossMark files (gloss onboard, issue #257). It chains what the other
// onboarding packages already do: scaffold plans the config, tier files and
// one manifest per deployable directory; importers reads a glossary or an
// enums file; harvest clusters the vocabulary the code, schemas, tests, UI
// and docs use; and this package turns the clusters into proposed concepts,
// reads the deploy manifests, Terraform and package manifests into a draft
// boundary map, and writes it all as proposals (proposed: true on every
// concept, a commented realizes: on every manifest) for gloss review.
// Markers are drafted afterwards by suggest, with proposed targets enabled.
package bootstrap

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/harvest"
	"github.com/mckalvan/glossmark/internal/importers"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/ontoedit"
	"github.com/mckalvan/glossmark/internal/scaffold"
)

// Options tunes a draft.
type Options struct {
	// Name, Core, Impl and BoundaryPrefix are passed to scaffold when the
	// repository has no glossmark.yaml yet.
	Name, Core, Impl, BoundaryPrefix string
	// Top caps the core concepts drafted from harvest clusters (default
	// DefaultTop); the implementation tier takes at most half as many.
	Top int
	// By is written as proposed-by on every concept.
	By string
	// Git lets harvest read the history for renames and dates.
	Git bool
}

// DefaultTop is how many core candidates become draft concepts: enough to
// recognise the domain in the viewer, few enough to review in one sitting.
const DefaultTop = 40

// Workload is a deployable found in a deploy manifest: a Kubernetes
// workload or a Compose service with a build.
type Workload struct {
	Name, Kind, Path string
	// Dir is the scaffold directory the workload's name matched (the
	// service's source), "" when none did.
	Dir string
	// Concept is the implementation system-type drafted for it.
	Concept string
}

// Resource is a Terraform resource worth a concept (a queue, a database, a
// bucket), not one that only supports another (a role, a subnet group).
type Resource struct {
	Type, Name, Path, Concept string
}

// Result reports what the draft wrote.
type Result struct {
	Root     string
	Core     string
	Impl     string
	Boundary string
	CoreFile string
	ImplFile string
	// Scaffolded is set when the repository had no glossmark.yaml and
	// scaffold wrote one; Plan is then its plan.
	Scaffolded bool
	Plan       *scaffold.Plan
	// Vocabulary lists the glossary and enums files imported, with counts.
	Vocabulary []string
	// Observations and Candidates are harvest's counts.
	Observations, Candidates int
	// CoreConcepts and ImplConcepts are the names drafted into each tier.
	CoreConcepts, ImplConcepts []string
	// SkippedExisting counts candidates already in the ontology (a re-run,
	// or a term the vocabulary import wrote first).
	SkippedExisting int
	Workloads       []Workload
	Resources       []Resource
	// Manifests lists the manifest files given a draft realizes:.
	Manifests []string
}

// Run drafts the ontology for root. It never overwrites: an existing
// concept is skipped, a manifest that already declares realizes: is left
// alone, and scaffold skips every file that exists.
func Run(root string, opt Options) (*Result, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if opt.Top <= 0 {
		opt.Top = DefaultTop
	}
	res := &Result{Root: root}
	if err := res.scaffold(opt); err != nil {
		return nil, err
	}
	if err := res.vocabulary(opt.By); err != nil {
		return nil, err
	}
	clusters, err := res.harvest(opt)
	if err != nil {
		return nil, err
	}
	res.boundaries()
	if err := res.concepts(clusters, opt); err != nil {
		return nil, err
	}
	if err := res.manifests(); err != nil {
		return nil, err
	}
	return res, nil
}

// scaffold writes the files gloss init would, or reads the namespaces of a
// workspace that already has them.
func (r *Result) scaffold(opt Options) error {
	cfgPath := filepath.Join(r.Root, load.ConfigName)
	if _, err := os.Stat(cfgPath); err != nil {
		plan, err := scaffold.Make(r.Root, scaffold.Options{Name: opt.Name, Core: opt.Core, Impl: opt.Impl, BoundaryPrefix: opt.BoundaryPrefix})
		if err != nil {
			return err
		}
		if err := plan.Apply(); err != nil {
			return err
		}
		r.Scaffolded, r.Plan = true, plan
	}
	cfg, err := model.LoadConfig(cfgPath)
	if err != nil {
		return err
	}
	for _, prefix := range sortedKeys(cfg.Owns) {
		ns := cfg.Owns[prefix]
		switch ns.Tier {
		case model.TierCore:
			if r.Core == "" {
				r.Core, r.CoreFile = prefix, filepath.Join(r.Root, filepath.FromSlash(ns.File))
			}
		case model.TierImpl:
			if r.Impl == "" {
				r.Impl, r.ImplFile = prefix, filepath.Join(r.Root, filepath.FromSlash(ns.File))
			}
		}
	}
	if r.Core == "" || r.Impl == "" {
		return fmt.Errorf("%s: onboard needs an owned core and an owned implementation namespace (a repository that imports its core is set up with gloss init --import)", load.ConfigName)
	}
	if r.Plan != nil {
		r.Boundary = r.Plan.Boundary
	} else {
		r.Boundary = r.Core + "b"
		if m, _, err := model.LoadManifest(filepath.Join(r.Root, load.ManifestName)); err == nil {
			if i := strings.Index(m.Boundary, ":"); i > 0 {
				r.Boundary = m.Boundary[:i]
			}
		}
	}
	return nil
}

// vocabulary imports a glossary or an enums file found in the tree into
// the core tier, every entry proposed. A re-run is a no-op through the
// import record, as gloss import is.
func (r *Result) vocabulary(by string) error {
	glossaries, enums := scaffold.VocabularyFiles(r.Root)
	if len(glossaries)+len(enums) == 0 {
		return nil
	}
	rec, err := importers.OpenRecord(filepath.Join(r.Root, load.StateDir, importers.RecordName))
	if err != nil {
		return err
	}
	apply := func(rel string, entries []importers.Entry) error {
		for i := range entries {
			entries[i].Proposed = true
		}
		res, err := importers.ApplyRecorded(r.CoreFile, r.Core, entries, by, false, rec)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		r.Vocabulary = append(r.Vocabulary, fmt.Sprintf("%s (%d added, %d already there)", rel, len(res.Added), len(res.Unchanged)+len(res.Updated)))
		return nil
	}
	for _, g := range glossaries {
		b, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(g)))
		if err != nil {
			return err
		}
		if err := apply(g, importers.ParseGlossary(string(b))); err != nil {
			return err
		}
	}
	for _, e := range enums {
		b, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(e)))
		if err != nil {
			return err
		}
		entries, _, err := importers.ParseEnumsWith(b, importers.EnumOptions{Values: true})
		if err != nil {
			return fmt.Errorf("%s: %w", e, err)
		}
		if err := apply(e, entries); err != nil {
			return err
		}
	}
	return rec.Save()
}

func (r *Result) harvest(opt Options) ([]harvest.Cluster, error) {
	ev, err := harvest.Harvest(r.Root, harvest.Options{Git: opt.Git})
	if err != nil {
		return nil, err
	}
	cs := harvest.Clusters(ev)
	r.Observations, r.Candidates = len(ev), len(cs)
	return cs, nil
}

// stopNames are single words that name a mechanism or a transport, not a
// domain concept, when they are all a candidate has (harvest strips them
// only as affixes of a longer name).
var stopNames = map[string]bool{"service": true, "services": true, "worker": true, "workers": true, "relay": true, "sender": true, "page": true, "pages": true,
	"web": true, "api": true, "apis": true, "session": true, "sessions": true, "scheduler": true, "database": true, "db": true, "url": true, "config": true,
	"configs": true, "status": true, "statuses": true, "type": true, "types": true, "handler": true, "handlers": true, "app": true, "apps": true, "server": true,
	"client": true, "job": true, "jobs": true, "task": true, "tasks": true, "message": true, "messages": true, "request": true, "requests": true,
	"response": true, "responses": true, "event": true, "events": true, "model": true, "models": true, "item": true, "items": true, "value": true,
	"values": true, "field": true, "fields": true, "record": true, "records": true, "row": true, "rows": true, "entry": true, "entries": true,
	"email": true, "sms": true, "sqs": true, "sns": true, "kafka": true, "topic": true, "topics": true, "queue": true, "queues": true, "table": true,
	"tables": true, "store": true, "cache": true, "test": true, "tests": true, "spec": true, "main": true, "index": true, "common": true, "shared": true,
	"core": true, "postgres": true, "redis": true, "mysql": true, "untested": true, "repository": true, "repositories": true, "kind": true, "route": true, "routes": true, "path": true, "label": true, "action": true, "actions": true, "util": true, "utils": true, "helper": true, "helpers": true, "shows": true, "show": true, "use": true, "uses": true, "new": true, "run": true}

// phraseWords mark a candidate as a phrase from a doc or the UI rather
// than a name: harvest keeps such runs when the code uses them too.
var phraseWords = map[string]bool{"the": true, "a": true, "an": true, "and": true, "or": true, "of": true, "to": true, "in": true, "on": true, "by": true,
	"with": true, "for": true, "at": true, "from": true, "when": true, "it": true, "what": true, "how": true, "if": true, "as": true, "is": true, "are": true, "not": true, "your": true, "my": true, "our": true, "this": true, "that": true}

// conceptName turns a cluster's name into a concept name: singular,
// PascalCase, or "" when it names no concept worth drafting.
func conceptName(name string) string {
	ws := splitWords(name)
	if len(ws) == 0 || len(ws) > 4 {
		return ""
	}
	if len(ws) == 1 && stopNames[strings.ToLower(ws[0])] {
		return ""
	}
	for _, w := range ws {
		if phraseWords[strings.ToLower(w)] {
			return "" // "join the waitlist", "booking and appointment": a sentence, not a name
		}
	}
	// An engineering affix is not part of the name (CirculationService is
	// the circulation; a bare one names nothing), and only the head noun
	// is singularised: "late cancellations".
	for len(ws) > 1 && aliasAffixes[strings.ToLower(ws[len(ws)-1])] {
		ws = ws[:len(ws)-1]
	}
	if len(ws) == 1 && (aliasAffixes[strings.ToLower(ws[0])] || stopNames[strings.ToLower(ws[0])]) {
		return ""
	}
	last := ws[len(ws)-1]
	if strings.ToUpper(last) != last || len(last) <= 3 {
		ws[len(ws)-1] = singular(last)
	}
	n := importers.Pascal(strings.Join(ws, " "))
	if len(n) < 3 || !unicode.IsLetter(rune(n[0])) {
		return ""
	}
	for _, c := range n {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return ""
		}
	}
	return n
}

// singular drops a plural ending: appointments, policies, no-shows, buses.
func singular(w string) string {
	lw := strings.ToLower(w)
	switch {
	case strings.HasSuffix(lw, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(lw, "ses") || strings.HasSuffix(lw, "xes") || strings.HasSuffix(lw, "ches") || strings.HasSuffix(lw, "shes"):
		return w[:len(w)-2]
	case strings.HasSuffix(lw, "ss") || strings.HasSuffix(lw, "us") || strings.HasSuffix(lw, "is") || !strings.HasSuffix(lw, "s") || len(w) <= 3:
		return w
	}
	return w[:len(w)-1]
}

// kindFor guesses a cluster's kind: the name's ending first, then what
// named it (a table or a type is an entity, a topic an event, a rule
// sentence or a guard a constraint).
func kindFor(c harvest.Cluster, name string) string {
	if k := importers.GuessKind(name); k != "entity" {
		return k
	}
	shape := c.Signals["table"] + c.Signals["type-decl"] + c.Signals["schema-type"] + c.Signals["enum-type"]
	switch {
	case c.Signals["topic"] > 0 && shape == 0:
		return "event"
	case c.Signals["rule-sentence"] >= 2 && shape == 0:
		return "constraint"
	}
	return "entity"
}

// draft is one concept to write.
type draft struct {
	name    string
	tier    string
	cluster harvest.Cluster
}

// concepts turns the ranked clusters into proposed concepts: corroborated,
// docs-only and code-only-visible candidates into the core tier, code-only
// internal ones (named at least twice) into the implementation tier as
// internal mechanisms. A name the ontology already has is skipped.
func (r *Result) concepts(clusters []harvest.Cluster, opt Options) error {
	taken := map[string]bool{}
	for _, file := range []string{r.CoreFile, r.ImplFile} {
		for _, n := range existingNames(file) {
			taken[strings.ToLower(n)] = true
		}
	}
	aliasOf := existingAliases(r.CoreFile)
	// The system-types and resources are named first: "booking api" in
	// the docs is the service, not a core concept beside it.
	var newWorkloads, newResources []int
	for i := range r.Workloads {
		w := &r.Workloads[i]
		if !taken[strings.ToLower(w.Concept)] {
			taken[strings.ToLower(w.Concept)] = true
			newWorkloads = append(newWorkloads, i)
		}
		w.Concept = r.Impl + ":" + w.Concept
	}
	for i := range r.Resources {
		x := &r.Resources[i]
		if !taken[strings.ToLower(x.Concept)] {
			taken[strings.ToLower(x.Concept)] = true
			newResources = append(newResources, i)
		}
		x.Concept = r.Impl + ":" + x.Concept
	}
	var drafts []draft
	core, impl := 0, 0
	for _, c := range clusters {
		if core >= opt.Top && impl >= opt.Top/2 {
			break
		}
		name := conceptName(c.Name)
		if name == "" {
			continue
		}
		if taken[strings.ToLower(name)] || aliasOf[strings.ToLower(c.Name)] != "" {
			r.SkippedExisting++
			continue
		}
		tier := model.TierCore
		switch c.Category {
		case harvest.CodeOnlyInternal:
			if c.Naming < 3 || len(c.Families) < 2 || impl >= opt.Top/2 {
				continue
			}
			tier = model.TierImpl
			impl++
		default:
			if len(c.Families) < 2 && c.Naming < 2 || core >= opt.Top {
				continue
			}
			core++
		}
		taken[strings.ToLower(name)] = true
		drafts = append(drafts, draft{name: name, tier: tier, cluster: c})
	}
	units := map[string][]string{} // unit dir → core process names, for realizes:
	for _, d := range drafts {
		spec := ontoedit.ConceptSpec{Kind: kindFor(d.cluster, d.name), Doc: docFor(d.cluster), Aliases: aliasesFor(d.cluster, d.name), Proposed: true, ProposedBy: opt.By}
		file := r.CoreFile
		if d.tier == model.TierImpl {
			file, spec.Internal = r.ImplFile, true
		}
		if err := ontoedit.AddConcept(file, d.name, spec); err != nil {
			return err
		}
		if d.tier == model.TierImpl {
			r.ImplConcepts = append(r.ImplConcepts, d.name)
			continue
		}
		r.CoreConcepts = append(r.CoreConcepts, d.name)
		if len(d.cluster.Units) == 1 && (spec.Kind == "process" || spec.Kind == "capability" || spec.Kind == "measure") {
			units[d.cluster.Units[0]] = append(units[d.cluster.Units[0]], r.Core+":"+d.name)
		}
	}
	// The system-types: one per deployable, realizing the core processes
	// only its directory names (a draft; the reviewer corrects it).
	for _, i := range newWorkloads {
		w := r.Workloads[i]
		name := strings.TrimPrefix(w.Concept, r.Impl+":")
		spec := ontoedit.ConceptSpec{Kind: "system-type", Proposed: true, ProposedBy: opt.By,
			Doc: fmt.Sprintf("The %s %s (%s). Drafted by gloss onboard; say what it realizes, or internal: true.", w.Name, strings.ToLower(w.Kind), w.Path)}
		if w.Dir != "" {
			spec.Realizes = units[w.Dir]
			if len(spec.Realizes) > 6 {
				spec.Realizes = spec.Realizes[:6]
			}
		}
		if err := ontoedit.AddConcept(r.ImplFile, name, spec); err != nil {
			return err
		}
		r.ImplConcepts = append(r.ImplConcepts, name)
	}
	for _, i := range newResources {
		x := r.Resources[i]
		name := strings.TrimPrefix(x.Concept, r.Impl+":")
		spec := ontoedit.ConceptSpec{Kind: "resource", Internal: true, Proposed: true, ProposedBy: opt.By,
			Doc: fmt.Sprintf("The %s %s provisioned in %s. Drafted by gloss onboard.", x.Name, x.Type, x.Path)}
		if err := ontoedit.AddConcept(r.ImplFile, name, spec); err != nil {
			return err
		}
		r.ImplConcepts = append(r.ImplConcepts, name)
	}
	return nil
}

// docFor writes the one sentence a draft carries: where the term was seen,
// so the reviewer can replace it with what the term means.
func docFor(c harvest.Cluster) string {
	var fams []string
	for f := range c.Families {
		fams = append(fams, f)
	}
	sort.Strings(fams)
	return fmt.Sprintf("Draft: %s, seen %d times in %s (gloss harvest --about %q). Replace this with what it means.", c.Category, c.Naming+c.Mentions, strings.Join(fams, ", "), c.Name)
}

// aliasesFor lists the other written forms of a cluster, at most four,
// that differ from the name by more than case or separators.
func aliasesFor(c harvest.Cluster, name string) []string {
	seen := map[string]bool{strings.ToLower(name): true, strings.ToLower(c.Name): true}
	var out []string
	for _, s := range c.Surfaces {
		k := strings.ToLower(importers.Pascal(s))
		if seen[k] || k == "" || len(s) > 40 || strings.ContainsAny(s, "/{}<>:=") || affixed(s) || strings.ToLower(conceptName(s)) == strings.ToLower(name) {
			continue
		}
		seen[k] = true
		out = append(out, s)
		if len(out) == 4 {
			break
		}
	}
	return out
}

// aliasAffixes are the engineering words a written form adds to a name
// (PatientID, AppointmentRepo, BookRequest): not another name for it.
var aliasAffixes = map[string]bool{"id": true, "ids": true, "repo": true, "repository": true, "request": true, "response": true, "at": true, "row": true,
	"record": true, "model": true, "dto": true, "handler": true, "service": true, "manager": true, "controller": true, "table": true, "impl": true,
	"factory": true, "store": true, "client": true, "config": true, "props": true, "state": true, "context": true, "provider": true, "hook": true,
	"count": true, "url": true, "uri": true, "ms": true, "seconds": true, "list": true, "map": true, "set": true, "type": true, "kind": true,
	"util": true, "utils": true, "servlet": true, "helper": true, "helpers": true, "base": true, "abstract": true, "mixin": true, "dao": true, "api": true}

// affixed reports a written form that carries an engineering affix.
func affixed(s string) bool {
	for _, w := range strings.Fields(strings.ToLower(strings.Join(splitWords(s), " "))) {
		if aliasAffixes[w] {
			return true
		}
	}
	return false
}

var camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// splitWords splits a surface into words, CamelCase and snake_case included.
func splitWords(s string) []string {
	s = camelBoundary.ReplaceAllString(s, "$1 $2")
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// existingNames lists the concept names an ontology file holds.
func existingNames(file string) []string {
	o, err := model.LoadOntology(file)
	if err != nil {
		return nil
	}
	var out []string
	for n := range o.Concepts {
		out = append(out, n)
	}
	return out
}

// existingAliases maps each alias (lower-cased) to the concept that owns it.
func existingAliases(file string) map[string]string {
	out := map[string]string{}
	o, err := model.LoadOntology(file)
	if err != nil {
		return out
	}
	for n, c := range o.Concepts {
		for _, a := range c.Aliases {
			out[strings.ToLower(a)] = n
		}
	}
	return out
}

var (
	tfResource = regexp.MustCompile(`^\s*resource\s+"([a-z0-9_]+)"\s+"([^"]+)"`)
	// supporting resources only exist for another resource's sake
	// (suggest leaves them out of provisions too).
	tfSupport = regexp.MustCompile(`(^|_)(iam|role|policy|attachment|subnet|security_group|parameter|log_group|secret|key|association|rule|profile|version|versioning|identity|alias|permission|tag|option_group|dlq|definition|target|configuration|lifecycle|notification|public_access_block|encryption|acl|ownership|cors|listener|record|zone|certificate|provider|membership)(_|$)`)
	// composeFile names a Compose file.
	composeFile   = regexp.MustCompile(`^(docker-)?compose(\.[\w-]+)?\.ya?ml$`)
	workloadKinds = map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true, "CronJob": true, "Job": true, "Rollout": true}
)

// boundaries reads the deploy manifests (Kubernetes workloads, Compose
// services with a build) and the Terraform resources under the scaffold
// directories, names a system-type per workload and a resource per
// resource, and matches each workload to the directory whose name it
// shares (its source).
func (r *Result) boundaries() {
	dirs := scaffold.SourceDirs(r.Root)
	seenWorkload := map[string]bool{}
	filepath.WalkDir(r.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(r.Root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p != r.Root && (strings.HasPrefix(d.Name(), ".") || load.SkipDir(d.Name()) || d.Name() == load.StateDir) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		ext := strings.ToLower(path.Ext(name))
		if ext != ".yaml" && ext != ".yml" && ext != ".tf" {
			return nil
		}
		if info, err := d.Info(); err != nil || info.Size() > load.MaxFileSize {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(b), "\n")
		switch {
		case ext == ".tf":
			for _, l := range lines {
				m := tfResource.FindStringSubmatch(l)
				if m == nil || tfSupport.MatchString(m[1]) || tfSupport.MatchString(m[2]) || strings.HasPrefix(m[1], "random_") || strings.HasPrefix(m[1], "null_") || strings.HasPrefix(m[1], "local_") {
					continue
				}
				r.Resources = append(r.Resources, Resource{Type: m[1], Name: m[2], Path: rel, Concept: resourceName(m[1], m[2])})
			}
		case composeFile.MatchString(name):
			for _, n := range composeServices(lines) {
				if !seenWorkload[n] {
					seenWorkload[n] = true
					r.Workloads = append(r.Workloads, Workload{Name: n, Kind: "Compose service", Path: rel})
				}
			}
		default:
			for _, w := range k8sWorkloads(lines) {
				if !seenWorkload[w.Name] {
					seenWorkload[w.Name] = true
					w.Path = rel
					r.Workloads = append(r.Workloads, w)
				}
			}
		}
		return nil
	})
	for i := range r.Workloads {
		w := &r.Workloads[i]
		w.Dir = matchDir(w.Name, dirs)
		w.Concept = systemTypeName(w.Name)
	}
	// A deployable directory no manifest names still runs something: a
	// system-type of its own, so the map has a box with a service in it.
	// The directories that hold the deploy manifests or the Terraform
	// realize what they run or provision instead.
	named := map[string]bool{}
	for _, w := range r.Workloads {
		named[w.Dir] = true
	}
	for _, d := range dirs {
		n := path.Base(d)
		if named[d] || hasWorkloads(r, d) || hasResources(r, d) || seenWorkload[n] || !hasCode(filepath.Join(r.Root, filepath.FromSlash(d))) {
			continue
		}
		seenWorkload[n] = true
		r.Workloads = append(r.Workloads, Workload{Name: n, Kind: "package", Path: d, Dir: d, Concept: systemTypeName(n)})
	}
	sort.Slice(r.Resources, func(i, j int) bool { return r.Resources[i].Concept < r.Resources[j].Concept })
	sort.SliceStable(r.Workloads, func(i, j int) bool { return r.Workloads[i].Concept < r.Workloads[j].Concept })
}

// codeExts are the languages a program is written in: a boundary holding
// one runs something, a boundary of schemas or configuration does not.
var codeExts = map[string]bool{".py": true, ".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".java": true, ".kt": true, ".cs": true,
	".rs": true, ".rb": true, ".pl": true, ".php": true, ".scala": true, ".swift": true, ".ex": true, ".exs": true, ".dart": true, ".c": true, ".cc": true, ".cpp": true}

// hasCode reports whether dir holds a source file outside hidden and
// skipped directories.
func hasCode(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if p != dir && (strings.HasPrefix(d.Name(), ".") || load.SkipDir(d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if codeExts[strings.ToLower(filepath.Ext(d.Name()))] {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func hasWorkloads(r *Result, dir string) bool {
	for _, w := range r.Workloads {
		if w.Kind != "package" && under(w.Path, dir) {
			return true
		}
	}
	return false
}

func hasResources(r *Result, dir string) bool {
	for _, x := range r.Resources {
		if under(x.Path, dir) {
			return true
		}
	}
	return false
}

func under(rel, dir string) bool { return rel == dir || strings.HasPrefix(rel, dir+"/") }

// k8sWorkloads finds the workloads in a Kubernetes YAML file, one per
// document: kind and metadata.name.
func k8sWorkloads(lines []string) []Workload {
	var out []Workload
	kind, name := "", ""
	flush := func() {
		if workloadKinds[kind] && name != "" {
			out = append(out, Workload{Name: name, Kind: kind})
		}
		kind, name = "", ""
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "---":
			flush()
		case strings.HasPrefix(l, "kind:"):
			kind = strings.TrimSpace(strings.TrimPrefix(t, "kind:"))
		case strings.HasPrefix(t, "name:") && strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && name == "":
			name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "name:")), `"'`)
		}
	}
	flush()
	return out
}

// composeServices lists the services of a Compose file that are built from
// the repository (a build: key); an image-only service is a third-party
// dependency (a broker, a database), not a boundary of this system.
func composeServices(lines []string) []string {
	var out []string
	inServices := false
	cur := ""
	built := map[string]bool{}
	var order []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ind := len(l) - len(strings.TrimLeft(l, " "))
		switch {
		case ind == 0:
			inServices = strings.HasPrefix(t, "services:")
			cur = ""
		case inServices && ind == 2 && strings.HasSuffix(t, ":"):
			cur = strings.TrimSuffix(t, ":")
			order = append(order, cur)
		case inServices && cur != "" && ind == 4 && (strings.HasPrefix(t, "build:") || t == "build"):
			built[cur] = true
		}
	}
	for _, s := range order {
		if built[s] {
			out = append(out, s)
		}
	}
	return out
}

// matchDir finds the scaffold directory a workload is built from: the one
// whose base name shares the most words with the workload's name, and
// shares all of its own (reminders-worker runs services/reminders).
func matchDir(name string, dirs []string) string {
	wt := tokenSet(name)
	best, bestN := "", 0
	for _, d := range dirs {
		dt := tokenSet(path.Base(d))
		if len(dt) == 0 {
			continue
		}
		n := 0
		for t := range dt {
			if wt[t] {
				n++
			}
		}
		if n == len(dt) && n > bestN {
			best, bestN = d, n
		}
	}
	return best
}

func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range harvest.Tokens(s) {
		out[t] = true
	}
	return out
}

// systemTypeName names the system-type for a workload: PascalCase, with
// Service added unless the name already says what kind of program it is.
func systemTypeName(name string) string {
	n := importers.Pascal(name)
	for _, suffix := range []string{"Service", "Worker", "Api", "Job", "App", "Site", "Gateway", "Daemon", "Cron", "Server", "Web", "Ui", "Cli", "Batch", "Kiosk", "Frontend", "Backend", "Notifier", "Intake"} {
		if strings.HasSuffix(n, suffix) && n != suffix {
			return n
		}
	}
	return n + "Service"
}

// resourceName names the resource concept for a Terraform resource: the
// resource's name plus what the type says it is (reminder_events of
// aws_sqs_queue is ReminderEventsQueue).
func resourceName(typ, name string) string {
	n := importers.Pascal(name)
	parts := strings.Split(typ, "_")
	what := importers.Pascal(parts[len(parts)-1])
	switch what {
	case "Instance", "Cluster":
		if len(parts) > 2 {
			if w := importers.Pascal(parts[len(parts)-2]); w == "Db" || w == "Rds" {
				what = "Database"
			}
		}
	case "Db", "Rds":
		what = "Database"
	}
	if strings.HasSuffix(n, what) || what == "" {
		return n
	}
	return n + what
}

// manifests writes a draft realizes: into every container manifest that
// has none: the system-types the directory's workloads deploy, or the one
// drafted for it, or the resources its Terraform provisions.
func (r *Result) manifests() error {
	for _, d := range scaffold.SourceDirs(r.Root) {
		var curies []string
		for _, w := range r.Workloads {
			if w.Dir == d || w.Kind != "package" && under(w.Path, d) {
				curies = appendUnique(curies, w.Concept)
			}
		}
		for _, x := range r.Resources {
			if under(x.Path, d) {
				curies = appendUnique(curies, x.Concept)
			}
		}
		if len(curies) == 0 {
			continue
		}
		mp := filepath.Join(r.Root, filepath.FromSlash(d), load.ManifestName)
		b, err := os.ReadFile(mp)
		if err != nil {
			continue // no manifest: scaffold skipped the directory, or it is grouped
		}
		if strings.Contains(string(b), "\nrealizes:") || strings.HasPrefix(string(b), "realizes:") {
			continue
		}
		sort.Strings(curies)
		out := string(b)
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += "# gloss onboard: a draft of what runs here; the concepts are proposed. Correct it, or delete the line.\n"
		out += "realizes: [" + strings.Join(curies, ", ") + "]\n"
		if err := os.WriteFile(mp, []byte(out), 0o644); err != nil {
			return err
		}
		r.Manifests = append(r.Manifests, d+"/"+load.ManifestName)
	}
	return nil
}

func appendUnique(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
