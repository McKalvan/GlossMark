// Package ledger maintains .glossmark/ledger.jsonl: provenance and review
// state for every marker, keyed by marker id. Git blame is the second trail.
package ledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/version"
)

// FileName is the ledger file under .glossmark/.
const FileName = "ledger.jsonl"

// header is the first line of the ledger file.
type header struct {
	Type      string `json:"type"`
	Format    int    `json:"format"`
	Generator string `json:"generator"`
}

// Ledger is the in-memory ledger.
type Ledger struct {
	Path    string
	Entries map[string]*model.LedgerEntry
	// Actors, when set, names the actor of a marker that has no entry yet.
	// Without it every new entry is attributed to the actor passed to Sync.
	Actors *Actors
	dirty  bool
}

// Actors resolves the actor of a marker seen for the first time by this
// ledger. The actor belongs to the marker's first appearance, not to whoever
// runs the build, so the order is: the actor the last built graph recorded
// for the same marker id, then the session's GLOSS_ACTOR, then the git blame
// author of the marker line when it is committed, then the fallback
// (human:<git user>). Existing ledger entries always keep their actor.
type Actors struct {
	// Previous maps marker id → actor from the last built graph.
	Previous map[string]string
	// Session is GLOSS_ACTOR, or "" when unset.
	Session string
	// Root is the repository root for git blame; "" disables blame.
	Root string
	// Fallback is used when nothing else applies.
	Fallback string

	blame map[string]map[int]string // path → line → actor, lazily filled
}

// PreviousActors collects marker id → actor from a built graph's inline edges.
func PreviousActors(g *graph.Graph) map[string]string {
	out := map[string]string{}
	if g == nil {
		return out
	}
	for _, e := range g.Edges() {
		if e.Source == graph.SourceInline && e.Prov != nil && e.Prov.ID != "" && e.Prov.Actor != "" {
			out[e.Prov.ID] = e.Prov.Actor
		}
	}
	return out
}

// For returns the actor for a marker without a ledger entry.
func (a *Actors) For(fm extract.FoundMarker) string {
	if a == nil {
		return ""
	}
	if prev := a.Previous[fm.ID]; prev != "" {
		return prev
	}
	if a.Session != "" {
		return a.Session
	}
	if b := a.blameActor(fm.Path, fm.Marker.Line); b != "" {
		return b
	}
	return a.Fallback
}

// blameActor returns human:<author> for a committed line, "" otherwise.
func (a *Actors) blameActor(path string, line int) string {
	if a.Root == "" || line <= 0 {
		return ""
	}
	if a.blame == nil {
		a.blame = map[string]map[int]string{}
	}
	lines, ok := a.blame[path]
	if !ok {
		lines = blameFile(a.Root, path)
		a.blame[path] = lines
	}
	return lines[line]
}

// blameFile runs git blame --line-porcelain once per file and maps every
// committed line to its author. Uncommitted lines (hash 0000...) map to "".
func blameFile(root, path string) map[int]string {
	out := map[int]string{}
	b, err := exec.Command("git", "-C", root, "blame", "--line-porcelain", "--", filepath.FromSlash(path)).Output()
	if err != nil {
		return out
	}
	line, uncommitted := 0, false
	for _, l := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(l, "\t"):
			continue
		case strings.HasPrefix(l, "author "):
			if !uncommitted && line > 0 {
				out[line] = humanActor(strings.TrimPrefix(l, "author "))
			}
		default:
			// Header: <hash> <orig> <final> [<count>]
			f := strings.Fields(l)
			if len(f) >= 3 && len(f[0]) == 40 {
				n := 0
				fmt.Sscanf(f[2], "%d", &n)
				line = n
				uncommitted = strings.Trim(f[0], "0") == ""
			}
		}
	}
	return out
}

func humanActor(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return "human:" + strings.ReplaceAll(strings.ToLower(name), " ", "-")
}

// Open reads the ledger; a missing file is an empty ledger.
func Open(path string) (*Ledger, error) {
	l := &Ledger{Path: path, Entries: map[string]*model.LedgerEntry{}}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return l, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if n == 1 && strings.Contains(line, `"type":"ledger"`) {
			var h header
			if err := json.Unmarshal([]byte(line), &h); err == nil && h.Format > version.LedgerFormat {
				return nil, fmt.Errorf("%s: format %d was written by a newer gloss (%s); this release reads format %d", path, h.Format, h.Generator, version.LedgerFormat)
			}
			continue
		}
		var e model.LedgerEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		if e.MarkerID == "" {
			continue
		}
		l.Entries[e.MarkerID] = &e
	}
	return l, sc.Err()
}

// Save writes the ledger sorted by marker id, if anything changed.
func (l *Ledger) Save() error {
	if !l.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	ids := make([]string, 0, len(l.Entries))
	for id := range l.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var sb strings.Builder
	hb, _ := json.Marshal(header{Type: "ledger", Format: version.LedgerFormat, Generator: version.Generator()})
	sb.Write(hb)
	sb.WriteByte('\n')
	for _, id := range ids {
		b, err := json.Marshal(l.Entries[id])
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(l.Path, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	l.dirty = false
	return nil
}

// Sync reconciles the ledger with the markers found in this build and stamps
// actor provenance onto the graph. Review-state transitions:
//
//	new marker            → proposed (with ?) | accepted (without)
//	? removed in source   → accepted, reviewed_by = actor
//	? re-added            → proposed
//	fingerprint changed   → stale (accepted links only)
//	fingerprint restored  → stays stale until re-accepted
func (l *Ledger) Sync(g *graph.Graph, markers []extract.FoundMarker, actor string, now time.Time) {
	ts := now.Format(time.RFC3339)
	seen := map[string]bool{}
	for _, fm := range markers {
		seen[fm.ID] = true
		e, ok := l.Entries[fm.ID]
		if !ok {
			by := actor
			if a := l.Actors.For(fm); a != "" {
				by = a
			}
			e = &model.LedgerEntry{MarkerID: fm.ID, Path: fm.Path, Relation: fm.Marker.Relation, Targets: fm.Marker.Targets,
				Actor: by, Model: modelOf(by), Created: ts, AnchorFingerprint: fm.Anchor.Fingerprint, Rationale: fm.Marker.Keys["note"]}
			if c := fm.Marker.Conf(); c >= 0 {
				e.Conf = c
			}
			if fm.Marker.Proposed {
				e.Status = model.StatusProposed
			} else {
				e.Status = model.StatusAccepted
				if !strings.HasPrefix(actor, "agent:") {
					e.ReviewedBy, e.ReviewedAt = actor, ts
				}
			}
			l.Entries[fm.ID] = e
			l.dirty = true
		} else {
			prev := *e
			e.Path, e.Relation, e.Targets = fm.Path, fm.Marker.Relation, fm.Marker.Targets
			if c := fm.Marker.Conf(); c >= 0 {
				e.Conf = c
			}
			if n := fm.Marker.Keys["note"]; n != "" {
				e.Rationale = n
			}
			switch {
			case fm.Marker.Proposed && e.Status != model.StatusProposed && e.Status != model.StatusRejected:
				e.Status = model.StatusProposed
			case !fm.Marker.Proposed && e.Status == model.StatusProposed:
				e.Status = model.StatusAccepted
				e.ReviewedBy, e.ReviewedAt = actor, ts
				e.AnchorFingerprint = fm.Anchor.Fingerprint
			case !fm.Marker.Proposed && e.Status == model.StatusAccepted && e.AnchorFingerprint != "" && e.AnchorFingerprint != fm.Anchor.Fingerprint:
				e.Status = model.StatusStale
			case e.Status == model.StatusRejected && !fm.Marker.Proposed:
				// A rejected link re-asserted without ? is a human decision.
				e.Status = model.StatusAccepted
				e.ReviewedBy, e.ReviewedAt = actor, ts
				e.AnchorFingerprint = fm.Anchor.Fingerprint
			}
			if e.AnchorFingerprint == "" {
				e.AnchorFingerprint = fm.Anchor.Fingerprint
			}
			if !equalEntries(e, &prev) {
				l.dirty = true
			}
		}
		e.LastSeen = ts
		for _, ed := range fm.Edges {
			if ed.Prov != nil {
				ed.Prov.Actor = e.Actor
			}
		}
		if e.Actor != "" {
			g.AddNode(&graph.Node{NodeType: model.NodeActor, ID: "urn:gloss:actor:" + e.Actor, CURIE: e.Actor, Label: e.Actor, Kind: strings.SplitN(e.Actor, ":", 2)[0]})
		}
	}
	// Proposed concepts are tracked like markers, keyed concept:<curie>.
	for _, n := range g.NodesOf(model.NodeConcept) {
		if n.Imported {
			continue
		}
		id := "concept:" + n.CURIE
		e, ok := l.Entries[id]
		switch {
		case n.Proposed && !ok:
			by := n.ProposedBy
			if by == "" {
				by = actor
			}
			l.Entries[id] = &model.LedgerEntry{MarkerID: id, Path: n.File, Relation: "concept", Targets: []string{n.CURIE}, Actor: by, Model: modelOf(by), Created: ts, Status: model.StatusProposed, Rationale: n.Doc, LastSeen: ts}
			l.dirty = true
		case n.Proposed && ok:
			if e.Status == model.StatusRemoved {
				e.Status = model.StatusProposed
				l.dirty = true
			}
			e.LastSeen = ts
		case !n.Proposed && ok && e.Status == model.StatusProposed:
			e.Status = model.StatusAccepted
			e.ReviewedBy, e.ReviewedAt = actor, ts
			e.LastSeen = ts
			l.dirty = true
		case ok:
			e.LastSeen = ts
		}
		if ok || n.Proposed {
			seen[id] = true
		}
	}
	for id, e := range l.Entries {
		if !seen[id] && e.Status != model.StatusRemoved {
			e.Status = model.StatusRemoved
			l.dirty = true
		}
	}
}

func equalEntries(a, b *model.LedgerEntry) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	// LastSeen differs every build; ignore it.
	return strings.Replace(string(ab), a.LastSeen, "", 1) == strings.Replace(string(bb), b.LastSeen, "", 1)
}

// Accept marks an entry accepted by reviewer.
func (l *Ledger) Accept(id, reviewer, fingerprint string, now time.Time) bool {
	e, ok := l.Entries[id]
	if !ok {
		return false
	}
	e.Status = model.StatusAccepted
	e.ReviewedBy, e.ReviewedAt = reviewer, now.Format(time.RFC3339)
	if fingerprint != "" {
		e.AnchorFingerprint = fingerprint
	}
	l.dirty = true
	return true
}

// Reject marks an entry rejected with a reason.
func (l *Ledger) Reject(id, reviewer, reason string, now time.Time) bool {
	e, ok := l.Entries[id]
	if !ok {
		return false
	}
	e.Status = model.StatusRejected
	e.ReviewedBy, e.ReviewedAt = reviewer, now.Format(time.RFC3339)
	if reason != "" {
		e.Rationale = reason
	}
	l.dirty = true
	return true
}

// Entry returns the entry for a marker id.
func (l *Ledger) Entry(id string) *model.LedgerEntry { return l.Entries[id] }

// ByStatus returns entries with a status, sorted by id.
func (l *Ledger) ByStatus(status string) []*model.LedgerEntry {
	var out []*model.LedgerEntry
	for _, e := range l.Entries {
		if e.Status == status {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MarkerID < out[j].MarkerID })
	return out
}

// Touch marks the ledger dirty (used by callers that mutate entries directly).
func (l *Ledger) Touch() { l.dirty = true }

func modelOf(actor string) string {
	if i := strings.Index(actor, "@"); i >= 0 && strings.HasPrefix(actor, "agent:") {
		return actor[i+1:]
	}
	return ""
}

// DefaultActor derives human:<git user.name> for a repository, else human:unknown.
func DefaultActor(root string) string {
	out, err := exec.Command("git", "-C", root, "config", "user.name").Output()
	name := strings.TrimSpace(string(out))
	if err != nil || name == "" {
		if u := os.Getenv("USER"); u != "" {
			return "human:" + u
		}
		return "human:unknown"
	}
	return humanActor(name)
}

// ActorChange is a marker whose actor differs from what the last built graph recorded.
type ActorChange struct {
	MarkerID, Path, From, To string
}

// ActorChanges lists markers whose actor this build would rewrite compared
// with previous (marker id → actor), sorted by marker id. A non-empty result
// means the ledger and the committed graph disagree; the ledger wins, and the
// disagreement usually means the ledger was not committed.
func (l *Ledger) ActorChanges(previous map[string]string) []ActorChange {
	var out []ActorChange
	for id, was := range previous {
		e, ok := l.Entries[id]
		if !ok || e.Status == model.StatusRemoved || e.Actor == "" || e.Actor == was {
			continue
		}
		out = append(out, ActorChange{MarkerID: id, Path: e.Path, From: was, To: e.Actor})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MarkerID < out[j].MarkerID })
	return out
}

// ExportPROV renders the ledger as PROV-O-shaped JSON-LD.
func (l *Ledger) ExportPROV() ([]byte, error) {
	type activity struct {
		ID        string `json:"@id"`
		Type      string `json:"@type"`
		Agent     string `json:"prov:wasAssociatedWith"`
		Generated string `json:"prov:generated"`
		At        string `json:"prov:startedAtTime"`
		Status    string `json:"gm:status"`
		Reviewer  string `json:"gm:reviewedBy,omitempty"`
	}
	var acts []activity
	ids := make([]string, 0, len(l.Entries))
	for id := range l.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := l.Entries[id]
		acts = append(acts, activity{ID: "urn:gloss:activity:" + id, Type: "prov:Activity", Agent: "urn:gloss:actor:" + e.Actor,
			Generated: "urn:gloss:marker:" + id, At: e.Created, Status: e.Status, Reviewer: e.ReviewedBy})
	}
	doc := map[string]any{
		"@context": map[string]string{"prov": "http://www.w3.org/ns/prov#", "gm": graph.VocabIRI},
		"@graph":   acts,
	}
	return json.MarshalIndent(doc, "", "  ")
}
