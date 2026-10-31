// @gloss implements gmi:SpecInference scope=file

package suggest

import (
	"regexp"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/infer"
	"github.com/mckalvan/glossmark/internal/marker"
)

// A source says that a declaration names a channel, and which of its
// messages; whether the code publishes to it or subscribes to it is code
// analysis, the same for every source, and done here: the line that
// names the channel, else the declaration's name, else what its calls do
// (#260, ADR 0041).

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

// declDir is the direction a declaration as a whole gives: what its name
// says (handle, publish), else what its own calls do. A declaration that
// says neither names the event for another reason (a rule reads its
// type) and gets no flow.
func declDir(d marker.Decl, body string) string {
	name := lastPart(d.Symbol)
	switch {
	case sendName.MatchString(name):
		return "publishes"
	case recvName.MatchString(name):
		return "subscribes"
	}
	consumer, producer := flow(d, body)
	switch {
	case producer && !consumer:
		return "publishes"
	case consumer && !producer:
		return "subscribes"
	}
	return ""
}

// flowRef is one channel a declaration names: the messages it names on
// it (none: the channel itself), what named each, and the direction.
type flowRef struct {
	channel  string
	messages map[string]infer.Hint // message name, or "" for the channel itself
	dir      string
}

// proposeFlows turns a source's flow hints for one declaration into
// publishes and subscribes proposals: one per message the code names on
// a channel, else one for the channel's own event. A flow is a link of
// its own beside whatever the declaration implements, so each is a
// proposal of its own.
func (r *runner) proposeFlows(f *file, d marker.Decl, role, source string, hints []infer.Hint) []Proposal {
	if len(hints) == 0 {
		return nil
	}
	refs := map[string]*flowRef{}
	for _, h := range hints {
		ref := refs[h.Channel]
		if ref == nil {
			ref = &flowRef{channel: h.Channel, messages: map[string]infer.Hint{}}
			refs[h.Channel] = ref
		}
		if _, ok := ref.messages[h.Message]; !ok {
			ref.messages[h.Message] = h
		}
		if ref.dir == "" && h.Line >= 1 && h.Line <= len(f.lines) {
			ref.dir = flowDir(f.lines[h.Line-1])
		}
	}
	fallback := declDir(d, bodyOf(f.lines, d))
	var out []Proposal
	for _, ref := range refs {
		if ref.dir == "" {
			ref.dir = fallback
		}
		if ref.dir == "" {
			continue
		}
		names := make([]string, 0, len(ref.messages))
		for name := range ref.messages {
			if name != "" {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		channelOnly := len(names) == 0
		if channelOnly {
			names = []string{""}
		}
		for _, name := range names {
			h := ref.messages[name]
			c := r.m.byCurie[h.Concept]
			if c == nil || !r.usable(f, c, ref.dir) {
				continue
			}
			basis := source + "/" + h.Basis + "/" + role
			terms := h.Terms
			if h.Via != "" && h.Via != terms {
				terms += "<" + h.Via
			}
			p := Proposal{Path: f.rel, Line: d.Insert, Symbol: d.Symbol, Relation: ref.dir, Target: r.shorten(f, c.curie), Matched: h.Basis + ":" + terms, Basis: basis, Source: source, Conf: confFor(basis), channel: ref.channel, channelOnly: channelOnly}
			if c.n.Deprecated {
				p.Deprecated, p.Note = true, deprecatedNote(c.n)
			}
			p.Marker = markerLine(f.style, d.Indent, p)
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].channel != out[j].channel {
			return out[i].channel < out[j].channel
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// plumbing drops the flow a declaration gets for only naming a channel
// when another declaration of the file tells that channel's messages
// apart in the same direction: subscribe([filed_topic, ...]) opens the
// subscription, handle(record_name) is where the events are handled, and
// the marker belongs there.
func plumbing(ps []Proposal) []Proposal {
	handled := map[string]bool{}
	for _, p := range ps {
		if p.channel != "" && !p.channelOnly {
			handled[p.channel+"\x00"+p.Relation] = true
		}
	}
	var out []Proposal
	for _, p := range ps {
		if p.channelOnly && handled[p.channel+"\x00"+p.Relation] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// flowHints splits a source's hints into the flows (a channel named) and
// the rest.
func flowHints(hs []infer.Hint) (flows, plain []infer.Hint) {
	for _, h := range hs {
		if h.Channel != "" {
			flows = append(flows, h)
		} else {
			plain = append(plain, h)
		}
	}
	return flows, plain
}

// shorten writes a target in the file's default namespace as .Name.
func (r *runner) shorten(f *file, curie string) string {
	if f.m != nil && f.m.DefaultNamespace != "" && strings.HasPrefix(curie, f.m.DefaultNamespace+":") {
		return "." + strings.TrimPrefix(curie, f.m.DefaultNamespace+":")
	}
	return curie
}
