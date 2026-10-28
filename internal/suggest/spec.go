// @gloss implements gmi:SpecInference scope=file

package suggest

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/load"
	"github.com/mckalvan/glossmark/internal/marker"
	"gopkg.in/yaml.v3"
)

// Teams already describe their domain in machine-readable form: an
// AsyncAPI contract names the events on every channel, an OpenAPI spec the
// operations and schemas of an API, a Strimzi KafkaTopic the topic it
// stands up, an ORM model the table it maps. The spec index reads them
// once per run so that the code which names a channel, a route or a table
// is proposed the link the spec implies (#260): a publishes or subscribes
// on the event a topic carries, an implements on the process an operation
// documents, a represents on the entity a table holds. Every inferred link
// is a proposal like any other: its conf is the precision its basis
// measured on the calibration set, so the reviewer sees how often the
// spec was right (ADR 0021).

// Sources a proposal's evidence can come from, named in Proposal.Source so
// gloss suggest --eval reports precision per source.
const (
	SourceCode     = "code"     // names, docstrings, tables touched, test calls
	SourceAsyncAPI = "asyncapi" // an AsyncAPI contract: its channels, and code that names them
	SourceOpenAPI  = "openapi"  // an OpenAPI spec: its operations and schemas, and routes that match them
	SourceRoute    = "route"    // a framework route with no spec behind it
	SourceTopic    = "topic"    // a Kafka topic resource (Strimzi KafkaTopic)
	SourceSchema   = "schema"   // a table, view or model declaration, or an ORM mapping to one
)

// Sources lists every source, for gloss suggest --source.
var Sources = []string{SourceCode, SourceAsyncAPI, SourceOpenAPI, SourceRoute, SourceTopic, SourceSchema}

// Roles of the items a spec file declares.
const (
	roleChannel   = "channel"   // an AsyncAPI channel (documents the event it carries)
	roleOperation = "operation" // an OpenAPI operation (documents the process it exposes)
	roleSchema    = "schema"    // an OpenAPI schema (represents the entity it shapes)
)

// channel is one AsyncAPI channel: the events it carries, resolved to
// concepts, and the one concept the channel stands for (the nearest
// common ancestor of its messages under is-a, else the event its name
// matches).
type channel struct {
	key, address string
	messages     []string // message names, as the spec writes them
	concepts     []*concept
	target       *concept
	// receivers are the consumer group ids of the receive operations;
	// sends and receives say which operations the spec declares.
	receivers       map[string]bool
	sends, receives bool
}

// operation is one OpenAPI operation, resolved to the process it documents.
type operation struct {
	method, path, id string
	norm             string // path with every parameter as {}
	target           *concept
	evidence         string
}

// specFile is a spec the index proposes on: its channels, operations and
// schemas as declarations.
type specFile struct {
	kind  string // asyncapi | openapi
	decls []marker.Decl
}

// alias is an identifier a boundary binds to topic addresses or message
// names (filed_topic = "claims.filed.v1", FILED_EVENT_TYPES = ("MotorClaimFiled", ...)).
type alias struct {
	addresses []string
	messages  []string
}

type specs struct {
	m        *matcher
	g        *graph.Graph
	channels map[string]*channel // by address and by channel key
	messages map[string]*concept // message name → event concept
	ops      []*operation
	files    map[string]*specFile
	// aliases are per boundary directory ("" for the root).
	aliases map[string]map[string]*alias
}

// specKind reports whether the first lines of a YAML or JSON file declare
// an AsyncAPI or OpenAPI document.
func specKind(lines []string) string {
	for i, l := range lines {
		if i > 30 {
			break
		}
		t := strings.TrimSpace(strings.Trim(strings.TrimSpace(l), `{,`))
		switch {
		case strings.HasPrefix(t, "asyncapi:"), strings.HasPrefix(t, `"asyncapi"`):
			return SourceAsyncAPI
		case strings.HasPrefix(t, "openapi:"), strings.HasPrefix(t, `"openapi"`), strings.HasPrefix(t, "swagger:"), strings.HasPrefix(t, `"swagger"`):
			return SourceOpenAPI
		}
	}
	return ""
}

// indexSpecs reads the workspace's specs and the identifiers its code binds
// to their names.
func indexSpecs(ws *load.Workspace, g *graph.Graph, m *matcher) *specs {
	s := &specs{m: m, g: g, channels: map[string]*channel{}, messages: map[string]*concept{}, files: map[string]*specFile{}, aliases: map[string]map[string]*alias{}}
	for _, rel := range ws.Files {
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		lines, err := readLines(ws.Abs(rel))
		if err != nil {
			continue
		}
		kind := specKind(lines)
		if kind == "" {
			continue
		}
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &doc); err != nil || len(doc.Content) == 0 {
			continue
		}
		root := doc.Content[0]
		sf := &specFile{kind: kind}
		switch kind {
		case SourceAsyncAPI:
			s.asyncAPI(root, lines, sf)
		case SourceOpenAPI:
			s.openAPI(root, lines, sf)
		}
		if ext != ".json" {
			s.files[rel] = sf
		}
	}
	if len(s.channels) > 0 {
		s.bindings(ws)
	}
	return s
}

// empty reports an index with nothing to infer from.
func (s *specs) empty() bool { return s == nil || len(s.channels) == 0 && len(s.ops) == 0 }

// asyncAPI indexes the channels, messages and operations of an AsyncAPI
// document (2.x or 3.x).
func (s *specs) asyncAPI(root *yaml.Node, lines []string, sf *specFile) {
	if chans := mapValue(root, "channels"); chans != nil && chans.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(chans.Content); i += 2 {
			k, v := chans.Content[i], chans.Content[i+1]
			ch := &channel{key: k.Value, address: k.Value}
			if a := mapValue(v, "address"); a != nil && a.Value != "" {
				ch.address = a.Value
			}
			if ms := mapValue(v, "messages"); ms != nil {
				ch.messages = append(ch.messages, messageNames(ms)...)
			}
			// AsyncAPI 2: the operations, and their messages, sit on the
			// channel.
			for _, op := range []string{"publish", "subscribe"} {
				if o := mapValue(v, op); o != nil {
					ch.messages = append(ch.messages, messageNames(mapValue(o, "message"))...)
				}
			}
			ch.messages = uniq(ch.messages)
			for _, name := range ch.messages {
				if c := s.eventNamed(name); c != nil {
					ch.concepts = append(ch.concepts, c)
					s.messages[name] = c
				}
			}
			ch.target = s.channelTarget(ch)
			s.channels[ch.address] = ch
			s.channels[ch.key] = ch
			first := k.Line
			sf.decls = append(sf.decls, marker.Decl{Symbol: ch.key, Kind: "channel", Line: first, End: blockEndAt(lines, first), Insert: first, Indent: indentOf(lines, first), Doc: scalar(mapValue(v, "description")) + " " + scalar(mapValue(v, "summary"))})
		}
	}
	// Messages declared in components but on no channel still name events
	// the code can reference.
	if ms := mapValue(mapValue(root, "components"), "messages"); ms != nil && ms.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(ms.Content); i += 2 {
			name := ms.Content[i].Value
			if n := mapValue(ms.Content[i+1], "name"); n != nil && n.Value != "" {
				name = n.Value
			}
			if _, ok := s.messages[name]; !ok {
				if c := s.eventNamed(name); c != nil {
					s.messages[name] = c
				}
			}
		}
	}
}

// messageNames lists the message names a messages map, a message or a
// oneOf list refers to: the last segment of a $ref, else the name field,
// else the key.
func messageNames(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	one := func(key string, v *yaml.Node) {
		if v == nil {
			return
		}
		if r := mapValue(v, "$ref"); r != nil {
			out = append(out, lastPart(r.Value))
			return
		}
		if alt := mapValue(v, "oneOf"); alt != nil {
			for _, a := range alt.Content {
				if r := mapValue(a, "$ref"); r != nil {
					out = append(out, lastPart(r.Value))
				} else if nm := mapValue(a, "name"); nm != nil {
					out = append(out, nm.Value)
				}
			}
			return
		}
		if nm := mapValue(v, "name"); nm != nil && nm.Value != "" {
			out = append(out, nm.Value)
			return
		}
		if key != "" {
			out = append(out, key)
		}
	}
	switch n.Kind {
	case yaml.MappingNode:
		if mapValue(n, "$ref") != nil || mapValue(n, "oneOf") != nil || mapValue(n, "payload") != nil || mapValue(n, "name") != nil {
			one("", n)
			return out
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			one(n.Content[i].Value, n.Content[i+1])
		}
	case yaml.SequenceNode:
		for _, v := range n.Content {
			one("", v)
		}
	}
	return out
}

// eventNamed resolves a message or channel name to the event concept it
// names exactly (name or alias), preferring the core tier.
func (s *specs) eventNamed(name string) *concept {
	var found []*concept
	for _, h := range s.m.nameHits(lastPart(name)) {
		if (h.kind == evNameExact || h.kind == evAliasExact) && h.c.n.Kind == "event" {
			found = append(found, h.c)
		}
	}
	if len(found) == 0 {
		return nil
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].impl != found[j].impl {
			return !found[i].impl
		}
		return found[i].curie < found[j].curie
	})
	return found[0]
}

// channelTarget is the concept a channel stands for: the nearest common
// is-a ancestor of its messages (claims.filed.v1 carries MotorClaimFiled,
// FloodClaimFiled and FireClaimFiled: ClaimFiled), else the event its key
// or address names.
func (s *specs) channelTarget(ch *channel) *concept {
	if len(ch.concepts) > 0 {
		if c := s.commonAncestor(ch.concepts); c != nil {
			return c
		}
	}
	for _, name := range []string{ch.key, stripVersion(ch.address)} {
		if c := s.eventNamed(name); c != nil {
			return c
		}
	}
	return nil
}

// commonAncestor finds the concept every one of cs is-a (itself included)
// that is nearest to them all.
func (s *specs) commonAncestor(cs []*concept) *concept {
	dist := func(c *concept) map[string]int {
		d := map[string]int{c.curie: 0}
		queue := []*graph.Node{c.n}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			for _, o := range s.g.Objects(n.ID, "is-a") {
				on := s.g.Node(o)
				if on == nil || on.CURIE == "" {
					continue
				}
				if _, seen := d[on.CURIE]; !seen {
					d[on.CURIE] = d[s.g.Curie(n.ID)] + 1
					queue = append(queue, on)
				}
			}
		}
		return d
	}
	common := dist(cs[0])
	for _, c := range cs[1:] {
		d := dist(c)
		for k, v := range common {
			if dv, ok := d[k]; ok {
				if dv > v {
					common[k] = dv
				}
			} else {
				delete(common, k)
			}
		}
	}
	best, bestD := "", -1
	for k, v := range common {
		if bestD < 0 || v < bestD || v == bestD && k < best {
			best, bestD = k, v
		}
	}
	return s.m.byCurie[best]
}

var versionSuffix = regexp.MustCompile(`[.\-_/]?v\d+$`)

func stripVersion(s string) string { return versionSuffix.ReplaceAllString(s, "") }

// openAPI indexes the operations and schemas of an OpenAPI (3.x) or
// Swagger (2.0) document.
func (s *specs) openAPI(root *yaml.Node, lines []string, sf *specFile) {
	if paths := mapValue(root, "paths"); paths != nil && paths.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(paths.Content); i += 2 {
			p, item := paths.Content[i], paths.Content[i+1]
			if item.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(item.Content); j += 2 {
				mk, op := item.Content[j], item.Content[j+1]
				if !httpMethods[strings.ToLower(mk.Value)] || op.Kind != yaml.MappingNode {
					continue
				}
				o := &operation{method: strings.ToLower(mk.Value), path: p.Value, norm: normPath(p.Value), id: scalar(mapValue(op, "operationId"))}
				doc := strings.TrimSpace(scalar(mapValue(op, "summary")) + " " + scalar(mapValue(op, "description")))
				if tags := mapValue(op, "tags"); tags != nil {
					for _, t := range tags.Content {
						doc += " " + t.Value
					}
				}
				o.target, o.evidence = s.processFor(o, doc)
				s.ops = append(s.ops, o)
				sym := o.id
				if sym == "" {
					sym = o.method + " " + o.path
				}
				first := mk.Line
				sf.decls = append(sf.decls, marker.Decl{Symbol: sym, Kind: "operation", Line: first, End: blockEndAt(lines, first), Insert: first, Indent: indentOf(lines, first), Doc: doc})
			}
		}
	}
	schemas := mapValue(mapValue(root, "components"), "schemas")
	if schemas == nil {
		schemas = mapValue(root, "definitions")
	}
	if schemas != nil && schemas.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(schemas.Content); i += 2 {
			k, v := schemas.Content[i], schemas.Content[i+1]
			first := k.Line
			sf.decls = append(sf.decls, marker.Decl{Symbol: k.Value, Kind: "schema", Line: first, End: blockEndAt(lines, first), Insert: first, Indent: indentOf(lines, first), Doc: scalar(mapValue(v, "description"))})
		}
	}
}

var httpMethods = map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true}

// processFor resolves an operation to the process or capability its id,
// path and summary name most strongly.
func (s *specs) processFor(o *operation, doc string) (*concept, string) {
	var hits []hit
	if o.id != "" {
		hits = append(hits, s.m.nameHits(o.id)...)
	}
	hits = append(hits, s.m.nameHits(pathWords(o.path))...)
	hits = append(hits, s.m.docHits(doc)...)
	var best hit
	for _, h := range hits {
		if !s.processLike(h.c) {
			continue
		}
		if best.c == nil || h.strength() > best.strength() || h.strength() == best.strength() && h.c.impl && !best.c.impl {
			best = h
		}
	}
	if best.c == nil {
		return nil, ""
	}
	return best.c, best.matched()
}

// processLike reports a concept an operation or a route can implement: a
// process or capability, or an implementation concept that realizes one.
func (s *specs) processLike(c *concept) bool {
	switch c.n.Kind {
	case "process", "capability":
		return true
	}
	for cur := range c.realizes {
		if r := s.m.byCurie[cur]; r != nil && (r.n.Kind == "process" || r.n.Kind == "capability") {
			return true
		}
	}
	return false
}

var pathParam = regexp.MustCompile(`\{[^}]*\}|:[A-Za-z_]\w*|<[^>]*>`)

// normPath writes every path parameter as {} and drops the trailing slash.
func normPath(p string) string {
	p = pathParam.ReplaceAllString(p, "{}")
	return strings.TrimSuffix(p, "/")
}

// pathWords are the literal segments of a path, parameters and version
// prefixes dropped, as one name.
func pathWords(p string) string {
	var out []string
	for _, seg := range strings.Split(normPath(p), "/") {
		if seg == "" || seg == "{}" || versionSuffix.MatchString("/"+seg) || seg == "api" {
			continue
		}
		out = append(out, seg)
	}
	return strings.Join(out, " ")
}

// scalar is a node's value, or "" for nil.
func scalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

// blockEndAt is the last line of the YAML block that starts at line (1-based):
// before the next non-blank line indented no deeper.
func blockEndAt(lines []string, line int) int {
	if line < 1 || line > len(lines) {
		return line
	}
	ind := len(indentOf(lines, line))
	end := line
	for k := line; k < len(lines); k++ {
		t := strings.TrimSpace(lines[k])
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if len(indentOf(lines, k+1)) <= ind {
			break
		}
		end = k + 1
	}
	return end
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

var (
	// assignment: an identifier bound to a value, in any of the languages
	// and config formats a service is written in (filed_topic: str =
	// "claims.filed.v1", const val FILED_TOPIC = "...", filedTopic =
	// "...", topic: payouts.v1).
	// The value is a literal, a list or tuple of literals, or a bare word
	// (YAML, properties); an expression or a keyword argument is not a
	// binding.
	assignment = regexp.MustCompile(`^\s*(?:(?:export|const|val|var|let|final|static|public|private|protected|readonly)\s+)*([A-Za-z_][\w.]*?)\s*(?::\s*[A-Za-z_][\w\[\], ]*)?\s*[:=]\s*((?:[\[(]\s*)?["'][A-Za-z][\w.\-/]*["'](?:\s*,\s*["'][A-Za-z][\w.\-/]*["'])*(?:\s*[\])])?|[A-Za-z][\w.\-]*)\s*;?\s*$`)
	quoted     = regexp.MustCompile(`["']([A-Za-z][\w.\-/]*)["']`)
)

// configExts are the configuration files read for topic aliases and
// consumer groups, on top of the code files suggest proposes in.
var configExts = map[string]bool{".conf": true, ".properties": true, ".toml": true, ".ini": true, ".env": true, ".json": true, ".cfg": true}

// bindings reads, per boundary, the identifiers the code and its
// configuration bind to the spec's addresses and message names. Tests
// bind fixtures, not the service's topics.
func (s *specs) bindings(ws *load.Workspace) {
	for _, rel := range ws.Files {
		ext := strings.ToLower(filepath.Ext(rel))
		if !codeExts[ext] && !configExts[ext] || ext == ".md" || ws.IsTestPath(rel) {
			continue
		}
		lines, err := readLines(ws.Abs(rel))
		if err != nil {
			continue
		}
		dir := ""
		if m := ws.ManifestFor(rel); m != nil {
			dir = m.Dir
		}
		for _, ln := range lines {
			m := assignment.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			vals := []string{m[2]}
			if q := quoted.FindAllStringSubmatch(m[2], -1); len(q) > 0 {
				vals = nil
				for _, v := range q {
					vals = append(vals, v[1])
				}
			}
			var a alias
			for _, v := range vals {
				switch {
				case s.channels[v] != nil:
					a.addresses = append(a.addresses, v)
				case s.messages[lastPart(v)] != nil:
					a.messages = append(a.messages, lastPart(v))
				}
			}
			if len(a.addresses) == 0 && len(a.messages) == 0 {
				continue
			}
			if s.aliases[dir] == nil {
				s.aliases[dir] = map[string]*alias{}
			}
			s.aliases[dir][lastPart(m[1])] = &a
		}
	}
}

// channelOf is the channel a message is declared on.
func (s *specs) channelOf(msg string) *channel {
	var found *channel
	for _, ch := range s.channels {
		for _, m := range ch.messages {
			if m == msg && (found == nil || ch.address < found.address) {
				found = ch
			}
		}
	}
	return found
}

// aliasFor finds an identifier's binding in a boundary, else at the root.
func (s *specs) aliasFor(dir, ident string) *alias {
	if a := s.aliases[dir][ident]; a != nil {
		return a
	}
	if dir != "" {
		return s.aliases[""][ident]
	}
	return nil
}

var (
	// Direction a line gives the topic it names: the call that sends to it
	// or reads from it.
	sendLine = regexp.MustCompile(`\.(?:to|send|produce|publish|emit|dispatch|_send|sendMessage|SendMessage|Publish|Produce|Emit|Send)\s*\(|ProducerRecord\s*\(|\bsend\s*\(|\b_send\s*\(`)
	recvLine = regexp.MustCompile(`\.(?:stream|table|subscribe|SubscribeTopics|Subscribe|consume|Consume|poll|receive|Receive|listen|on|handle|read_messages|ReadMessage|GlobalKTable|globalTable|from)\s*\(|@\w*(?:Listener|Incoming|Consumer|Subscribe|KafkaHandler)\b|\btopics\s*=`)
	// Direction a declaration's own name gives.
	sendName = regexp.MustCompile(`(?i)^(publish|send|emit|produce|dispatch|announce|notify|broadcast|push)`)
	recvName = regexp.MustCompile(`(?i)^(handle|consume|receive|listen|subscribe|on[A-Z_]|process|poll|read|ingest|react)`)
)

// flowDir is the direction a line of code gives a topic it names, else "".
func flowDir(line string) string {
	switch {
	case sendLine.MatchString(line):
		return "publishes"
	case recvLine.MatchString(line):
		return "subscribes"
	}
	return ""
}

// flowRef is one reference to a channel in a declaration: the messages it
// names on that channel (none: the channel itself), the identifier each
// was named through, and the direction.
type flowRef struct {
	ch       *channel
	messages map[string]*concept
	via      map[string]string // message name, or the address, → the identifier or literal that named it
	dir      string
}

// flows finds the AsyncAPI channels a declaration's body or decorators
// name, through an address, an alias or a message name, each with the
// direction the code gives it.
func (s *specs) flows(f *file, d marker.Decl, body string) []*flowRef {
	if s.empty() || len(s.channels) == 0 {
		return nil
	}
	dir := ""
	if f.m != nil {
		dir = f.m.Dir
	}
	refs := map[*channel]*flowRef{}
	ref := func(ch *channel, msg, via, ldir string) {
		r := refs[ch]
		if r == nil {
			r = &flowRef{ch: ch, messages: map[string]*concept{}, via: map[string]string{}}
			refs[ch] = r
		}
		if r.dir == "" {
			r.dir = ldir
		}
		if msg != "" {
			r.messages[msg] = s.messages[msg]
		} else {
			msg = ch.address
		}
		if _, ok := r.via[msg]; !ok {
			r.via[msg] = via
		}
	}
	channelOf := s.channelOf
	lines := append(append([]string{}, d.Decorators...), strings.Split(body, "\n")...)
	for _, ln := range lines {
		ldir := flowDir(ln)
		for _, q := range quoted.FindAllStringSubmatch(ln, -1) {
			v := q[1]
			if ch := s.channels[v]; ch != nil {
				ref(ch, "", v, ldir)
			} else if s.messages[lastPart(v)] != nil {
				if ch := channelOf(lastPart(v)); ch != nil {
					ref(ch, lastPart(v), lastPart(v), ldir)
				}
			}
		}
		for _, id := range ident.FindAllString(ln, -1) {
			if s.messages[id] != nil {
				if ch := channelOf(id); ch != nil {
					ref(ch, id, id, ldir)
				}
				continue
			}
			a := s.aliasFor(dir, id)
			if a == nil {
				continue
			}
			for _, addr := range a.addresses {
				ref(s.channels[addr], "", id, ldir)
			}
			for _, msg := range a.messages {
				if ch := channelOf(msg); ch != nil {
					ref(ch, msg, id, ldir)
				}
			}
		}
	}
	if len(refs) == 0 {
		return nil
	}
	// A reference no line gives a direction takes the declaration's: what
	// its name says (handle, publish), else what its own calls do. A
	// declaration that says neither names the event for another reason
	// (a rule reads its type) and gets no flow.
	declDir := ""
	name := lastPart(d.Symbol)
	switch {
	case sendName.MatchString(name):
		declDir = "publishes"
	case recvName.MatchString(name):
		declDir = "subscribes"
	}
	if declDir == "" {
		consumer, producer := flow(d, body)
		switch {
		case producer && !consumer:
			declDir = "publishes"
		case consumer && !producer:
			declDir = "subscribes"
		}
	}
	var out []*flowRef
	for _, r := range refs {
		if r.dir == "" {
			r.dir = declDir
		}
		if r.dir == "" {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ch.address < out[j].ch.address })
	return out
}

var (
	// routeCall: a framework route declaration, in a decorator or a call:
	// @app.route("/p", methods=["POST"]), @router.get("/p"), @GetMapping("/p"),
	// @PostMapping(value = "/p"), @Get('/p'), app.post('/p', ...), r.HandleFunc("/p", ...),
	// post("/p") { ... } (Ktor).
	routeCall    = regexp.MustCompile(`(?i)(?:@(?:\w+\.)?(route|get|post|put|patch|delete|GetMapping|PostMapping|PutMapping|PatchMapping|DeleteMapping|RequestMapping)|\b(?:\w+\.)?(get|post|put|patch|delete|route|HandleFunc|Handle|Get|Post|Put|Patch|Delete))\s*\(\s*(?:value\s*=\s*|path\s*=\s*)?["'](/[^"']*)["']`)
	routeMethods = regexp.MustCompile(`(?i)methods\s*=\s*[\[(]\s*["'](\w+)["']`)
)

// route is one route a declaration declares.
type route struct {
	method, path string
}

// routes finds the HTTP routes a declaration's decorators and body declare.
func routes(d marker.Decl, body string) []route {
	var out []route
	seen := map[route]bool{}
	for _, ln := range append(append([]string{}, d.Decorators...), strings.Split(body, "\n")...) {
		m := routeCall.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		method := strings.ToLower(m[1] + m[2])
		switch {
		case strings.HasSuffix(method, "mapping"):
			method = strings.TrimSuffix(method, "mapping")
			if method == "request" {
				method = ""
			}
		case method == "route", method == "handlefunc", method == "handle":
			method = ""
		}
		if mm := routeMethods.FindStringSubmatch(ln); mm != nil {
			method = strings.ToLower(mm[1])
		}
		r := route{method, m[3]}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// operationFor finds the spec operation a route declares: same method
// (when known) and a spec path that is, or ends with, the route's path.
func (s *specs) operationFor(r route) *operation {
	norm := normPath(r.path)
	var best *operation
	for _, o := range s.ops {
		if r.method != "" && o.method != r.method {
			continue
		}
		if o.norm == norm || strings.HasSuffix(o.norm, norm) && norm != "" && norm != "/" {
			if best == nil || len(o.norm) < len(best.norm) {
				best = o
			}
		}
	}
	return best
}
