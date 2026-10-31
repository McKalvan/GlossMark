// @gloss implements gmi:SpecInference scope=file

package infer

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/load"
	"gopkg.in/yaml.v3"
)

// asyncAPI reads AsyncAPI 2.x and 3.x contracts. A channel stands for one
// event concept: the nearest common is-a ancestor of the events its
// messages name (claims.filed.v1 carries MotorClaimFiled, FloodClaimFiled
// and FireClaimFiled: ClaimFiled), else the event its key or address
// names. The spec file gets a documents item per channel; code that names
// a channel, through its address, an identifier its boundary binds to it
// or a message name, gets a flow hint per message it tells apart, else
// one for the channel. Which way the flow goes is the runner's to decide.
type asyncAPI struct {
	channels map[string]*channel // by address and by channel key
	messages map[string]string   // message name → event CURIE
	files    map[string][]Item
	// aliases are per boundary directory ("" for the root): identifier →
	// the addresses and message names it is bound to.
	aliases map[string]map[string]*alias
}

type channel struct {
	key, address string
	messages     []string // message names, as the spec writes them
	target       string   // the concept the channel stands for, or ""
	basis        string   // messages | name
}

type alias struct {
	addresses []string
	messages  []string
}

func (s *asyncAPI) Name() string { return "asyncapi" }

func (s *asyncAPI) Index(ws *load.Workspace, v Vocabulary) error {
	s.channels, s.messages, s.files, s.aliases = map[string]*channel{}, map[string]string{}, map[string][]Item{}, map[string]map[string]*alias{}
	for _, rel := range ws.Files {
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		lines, root := readSpec(ws.Abs(rel), "asyncapi")
		if root == nil {
			continue
		}
		items := s.read(root, lines, v)
		if ext != ".json" {
			s.files[rel] = items
		}
	}
	if len(s.channels) > 0 {
		s.bindings(ws)
	}
	return nil
}

// read indexes one contract's channels and messages and returns its items.
func (s *asyncAPI) read(root *yaml.Node, lines []string, v Vocabulary) []Item {
	var items []Item
	if chans := mapValue(root, "channels"); chans != nil && chans.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(chans.Content); i += 2 {
			k, n := chans.Content[i], chans.Content[i+1]
			ch := &channel{key: k.Value, address: k.Value}
			if a := mapValue(n, "address"); a != nil && a.Value != "" {
				ch.address = a.Value
			}
			if ms := mapValue(n, "messages"); ms != nil {
				ch.messages = append(ch.messages, messageNames(ms)...)
			}
			// AsyncAPI 2: the operations, and their messages, sit on the
			// channel.
			for _, op := range []string{"publish", "subscribe"} {
				if o := mapValue(n, op); o != nil {
					ch.messages = append(ch.messages, messageNames(mapValue(o, "message"))...)
				}
			}
			ch.messages = uniq(ch.messages)
			var curies []string
			for _, name := range ch.messages {
				if c := eventNamed(v, name); c != "" {
					curies = append(curies, c)
					s.messages[name] = c
				}
			}
			if len(curies) > 0 {
				ch.target, ch.basis = v.CommonAncestor(curies), "messages"
			}
			if ch.target == "" {
				for _, name := range []string{ch.key, StripVersion(ch.address)} {
					if c := eventNamed(v, name); c != "" {
						ch.target, ch.basis = c, "name"
						break
					}
				}
			}
			s.channels[ch.address] = ch
			s.channels[ch.key] = ch
			doc := scalar(mapValue(n, "description")) + " " + scalar(mapValue(n, "summary"))
			items = append(items, item(lines, k.Line, ch.key, "channel", "documents", doc, Target{Concept: ch.target, Basis: ch.basis, Terms: strings.Join(ch.messages, ",")}))
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
				if c := eventNamed(v, name); c != "" {
					s.messages[name] = c
				}
			}
		}
	}
	return items
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
			out = append(out, LastPart(r.Value))
			return
		}
		if alt := mapValue(v, "oneOf"); alt != nil {
			for _, a := range alt.Content {
				if r := mapValue(a, "$ref"); r != nil {
					out = append(out, LastPart(r.Value))
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

// eventNamed resolves a message or channel name to the event it names
// exactly, or "".
func eventNamed(v Vocabulary, name string) string {
	if cs := v.Named(LastPart(name), "event"); len(cs) > 0 {
		return cs[0].CURIE
	}
	return ""
}

// bindings reads, per boundary, the identifiers the code and its
// configuration bind to the contract's addresses and message names.
// Tests bind fixtures, not the service's topics.
func (s *asyncAPI) bindings(ws *load.Workspace) {
	for _, rel := range ws.Files {
		ext := strings.ToLower(filepath.Ext(rel))
		if !codeExts[ext] && !configExts[ext] || ws.IsTestPath(rel) {
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
			b, ok := ParseBinding(ln)
			if !ok {
				continue
			}
			var a alias
			for _, v := range b.Values {
				switch {
				case s.channels[v] != nil:
					a.addresses = append(a.addresses, v)
				case s.messages[LastPart(v)] != "":
					a.messages = append(a.messages, LastPart(v))
				}
			}
			if len(a.addresses) == 0 && len(a.messages) == 0 {
				continue
			}
			if s.aliases[dir] == nil {
				s.aliases[dir] = map[string]*alias{}
			}
			s.aliases[dir][b.Ident] = &a
		}
	}
}

// aliasFor finds an identifier's binding in a boundary, else at the root.
func (s *asyncAPI) aliasFor(dir, id string) *alias {
	if a := s.aliases[dir][id]; a != nil {
		return a
	}
	if dir != "" {
		return s.aliases[""][id]
	}
	return nil
}

// channelOf is the channel a message is declared on.
func (s *asyncAPI) channelOf(msg string) *channel {
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

func (s *asyncAPI) Items(path string) []Item { return s.files[path] }

// Hints lists the channels a declaration names, line by line: through an
// address literal, a message name (as a literal, qualified or not, or an
// identifier), or an identifier its boundary binds to either.
func (s *asyncAPI) Hints(d Decl) []Hint {
	if len(s.channels) == 0 {
		return nil
	}
	var out []Hint
	type key struct{ channel, message string }
	seen := map[key]bool{}
	add := func(ch *channel, msg, via string, line int) {
		if ch == nil || seen[key{ch.address, msg}] {
			return
		}
		seen[key{ch.address, msg}] = true
		h := Hint{Channel: ch.address, Message: msg, Via: via, Line: line}
		h.Basis, h.Terms = "flow", msg
		if msg == "" {
			h.Concept, h.Terms = ch.target, ch.address
		} else {
			h.Concept = s.messages[msg]
		}
		if h.Concept == "" {
			return
		}
		out = append(out, h)
	}
	lines := append(append([]string{}, d.Decorators...), strings.Split(d.Body, "\n")...)
	for i, ln := range lines {
		line := d.Line
		if i >= len(d.Decorators) {
			line = d.Line + i - len(d.Decorators)
		}
		for _, q := range quoted.FindAllStringSubmatch(ln, -1) {
			v := q[1]
			if ch := s.channels[v]; ch != nil {
				add(ch, "", v, line)
			} else if msg := LastPart(v); s.messages[msg] != "" {
				add(s.channelOf(msg), msg, msg, line)
			}
		}
		for _, id := range ident.FindAllString(ln, -1) {
			if s.messages[id] != "" {
				add(s.channelOf(id), id, id, line)
				continue
			}
			a := s.aliasFor(d.Dir, id)
			if a == nil {
				continue
			}
			for _, addr := range a.addresses {
				add(s.channels[addr], "", id, line)
			}
			for _, msg := range a.messages {
				add(s.channelOf(msg), msg, id, line)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Channel != out[j].Channel {
			return out[i].Channel < out[j].Channel
		}
		return out[i].Message < out[j].Message
	})
	return out
}
