// @gloss represents gmi:ReviewLedger scope=file
// @gloss represents gm:Proposal scope=file

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
)

// FileName is the ledger file under .glossmark/.
const FileName = "ledger.jsonl"

// Ledger is the in-memory ledger.
type Ledger struct {
	Path    string
	Entries map[string]*model.LedgerEntry
	dirty   bool
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
		var e model.LedgerEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
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
			e = &model.LedgerEntry{MarkerID: fm.ID, Path: fm.Path, Relation: fm.Marker.Relation, Targets: fm.Marker.Targets,
				Actor: actor, Model: modelOf(actor), Created: ts, AnchorFingerprint: fm.Anchor.Fingerprint, Rationale: fm.Marker.Keys["note"]}
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
	return "human:" + strings.ReplaceAll(strings.ToLower(name), " ", "-")
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
