// @gloss represents gmi:ReviewLedger scope=file
// @gloss publishes gm:MappingStale scope=file

package ledger

import (
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/extract"
	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/model"
)

// SyncMappings reconciles the mapping entries (marker id mapping:<subject>|
// <predicate>|<object>) with the rows of the mapping sets (ADR 0025 §3-4).
// The file carries the review: a row without reviewer_id is proposed, one
// with it accepted, a reviewed predicate_modifier: Not row rejected. The
// ledger adds what the file cannot: both endpoints' concept fingerprints at
// review time, so a mapping whose endpoint changed since becomes stale until
// it is reviewed again (a later review_date, another reviewer, or gloss map
// accept). A rejection sticks, as for markers (ADR 0005), and a row that
// disappears is removed. It stamps each mapping edge with its review state.
func (l *Ledger) SyncMappings(g *graph.Graph, found []extract.FoundMapping, actor string, now time.Time) {
	ts := now.Format(time.RFC3339)
	seen := map[string]bool{}
	rows := append([]extract.FoundMapping{}, found...)
	// Positive rows first: when a set holds both a mapping and its negation
	// (GM704), the negation is the decision that stands.
	sort.SliceStable(rows, func(i, j int) bool { return !rows[i].Negative && rows[j].Negative })
	for _, fm := range rows {
		if !fm.Supported || len(fm.Unresolved) > 0 {
			continue
		}
		seen[fm.ID] = true
		r := fm.Row
		sfp, ofp := mapping.ConceptFingerprint(g, fm.Subject), mapping.ConceptFingerprint(g, fm.Object)
		reviewer := firstOf(r.Multi("reviewer_id"), r.Multi("reviewer_label"))
		reviewDate := r.Get("review_date")
		e, ok := l.Entries[fm.ID]
		if !ok {
			by := firstOf(r.Multi("author_id"), r.Multi("author_label"))
			if by == "" {
				by = actor
			}
			e = &model.LedgerEntry{MarkerID: fm.ID, Created: ts, Actor: by, Model: modelOf(by)}
			l.Entries[fm.ID] = e
			l.dirty = true
		}
		prev := *e
		e.Path, e.Relation, e.Targets, e.Set = fm.Set.Path, fm.Predicate.CURIE, []string{fm.SubjectCURIE, fm.ObjectCURIE}, fm.Set.Name
		if c := r.Confidence(); c >= 0 {
			e.Conf = c
		}
		if c := r.Get("comment"); c != "" {
			e.Rationale = c
		}
		review := func() {
			e.Status = model.StatusAccepted
			e.ReviewedBy = reviewer
			e.ReviewedAt = reviewDate
			if e.ReviewedAt == "" {
				e.ReviewedAt = ts
			}
			e.SubjectFingerprint, e.ObjectFingerprint = sfp, ofp
		}
		switch {
		case fm.Negative && r.Reviewed():
			e.Negative = true
			if e.Status != model.StatusRejected || reviewer != e.ReviewedBy {
				e.Status = model.StatusRejected
				e.ReviewedBy = reviewer
				e.ReviewedAt = reviewDate
				if e.ReviewedAt == "" {
					e.ReviewedAt = ts
				}
			}
			if e.ReviewReason == "" {
				e.ReviewReason = r.Get("comment")
			}
		case fm.Negative:
			// A proposed negation: an author suggests the pair does not match.
			e.Negative = true
			if e.Status != model.StatusRejected {
				e.Status = model.StatusProposed
			}
		case !r.Reviewed():
			e.Negative = false
			if e.Status != model.StatusRejected {
				e.Status = model.StatusProposed
			}
		case e.Status != model.StatusAccepted && e.Status != model.StatusStale:
			e.Negative = false
			review() // first review, or a rejected or removed pair reviewed again
		case reReviewed(e, reviewer, reviewDate):
			review()
		case e.Status == model.StatusAccepted:
			if e.SubjectFingerprint == "" {
				e.SubjectFingerprint = sfp
			}
			if e.ObjectFingerprint == "" {
				e.ObjectFingerprint = ofp
			}
			if changed(e.SubjectFingerprint, sfp) || changed(e.ObjectFingerprint, ofp) {
				e.Status = model.StatusStale
			}
		}
		if fm.Edge != nil && fm.Edge.Prov != nil {
			fm.Edge.Prov.Review = e.Status
			if fm.Edge.Prov.Actor == "" {
				fm.Edge.Prov.Actor = e.Actor
			}
		}
		e.LastSeen = prev.LastSeen
		if !equalEntries(e, &prev) {
			l.dirty = true
		}
		e.LastSeen = ts
	}
	for id, e := range l.Entries {
		if !strings.HasPrefix(id, "mapping:") || seen[id] {
			continue
		}
		if e.Status != model.StatusRemoved && e.Status != model.StatusRejected {
			e.Status = model.StatusRemoved
			l.dirty = true
		}
	}
}

// ReviewMapping records a review made with gloss map accept: it refreshes the
// fingerprints even when the file's review_date did not move (a mapping that
// went stale and is re-confirmed on the day it was first reviewed).
func (l *Ledger) ReviewMapping(id, reviewer, sfp, ofp string, now time.Time) {
	e, ok := l.Entries[id]
	if !ok {
		return
	}
	e.Status, e.Negative = model.StatusAccepted, false
	e.ReviewedBy, e.ReviewedAt = reviewer, now.Format(time.RFC3339)
	e.SubjectFingerprint, e.ObjectFingerprint = sfp, ofp
	l.dirty = true
}

// ReopenMapping undoes a rejection when the pair is proposed again on
// purpose (gloss map propose --force): the next sync sees a proposed row.
func (l *Ledger) ReopenMapping(id string) {
	e, ok := l.Entries[id]
	if !ok || e.Status != model.StatusRejected {
		return
	}
	e.Status, e.Negative = model.StatusProposed, false
	e.ReviewedBy, e.ReviewedAt, e.ReviewReason = "", "", ""
	l.dirty = true
}

// MappingEntries returns the mapping entries, sorted by id.
func (l *Ledger) MappingEntries() []*model.LedgerEntry {
	var out []*model.LedgerEntry
	for id, e := range l.Entries {
		if strings.HasPrefix(id, "mapping:") {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MarkerID < out[j].MarkerID })
	return out
}

func reReviewed(e *model.LedgerEntry, reviewer, reviewDate string) bool {
	if reviewer != "" && reviewer != e.ReviewedBy {
		return true
	}
	if reviewDate == "" {
		return false
	}
	recorded := e.ReviewedAt
	if len(recorded) > 10 {
		recorded = recorded[:10]
	}
	return reviewDate > recorded
}

func changed(recorded, current string) bool {
	return recorded != "" && current != "" && recorded != current
}

func firstOf(lists ...[]string) string {
	for _, l := range lists {
		for _, v := range l {
			if v != "" {
				return v
			}
		}
	}
	return ""
}
