// @gloss implements gmi:ReviewEngine scope=file

package review

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/mckalvan/glossmark/internal/load"
)

// RecommendationsFile holds, under .glossmark/, the decisions agents
// recommend and a human has not yet taken (ADR 0033). Commit it with the
// ledger so the reviewer sees them.
const RecommendationsFile = "recommendations.jsonl"

// Recommendation is a decision recorded for a human to take.
type Recommendation struct {
	Item     string   `json:"item"`
	Decision Decision `json:"decision"`
	By       string   `json:"by"`
	At       string   `json:"at"`
	// Basis is the item's basis when recommended; Outdated is set on read
	// when the item has changed since.
	Basis    string `json:"basis"`
	Outdated bool   `json:"outdated,omitempty"`
}

func readRecommendations(ws *load.Workspace) (map[string]Recommendation, error) {
	out := map[string]Recommendation{}
	b, err := os.ReadFile(stateFile(ws, RecommendationsFile))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r Recommendation
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", RecommendationsFile, n, err)
		}
		out[r.Item] = r
	}
	return out, sc.Err()
}

// writeRecommendations rewrites the file sorted by item, and removes it
// when nothing is left.
func writeRecommendations(ws *load.Workspace, recs map[string]Recommendation) error {
	path := stateFile(ws, RecommendationsFile)
	if len(recs) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	ids := make([]string, 0, len(recs))
	for id := range recs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var buf bytes.Buffer
	for _, id := range ids {
		r := recs[id]
		r.Outdated = false
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return writeFileAtomic(path, buf.Bytes())
}

func (e *Engine) recommend(st *State, it *Item, d Decision) error {
	recs, err := readRecommendations(st.WS)
	if err != nil {
		return err
	}
	d.Recommend = false
	recs[it.ID] = Recommendation{Item: it.ID, Decision: d, By: e.Actor, At: e.now().Format("2006-01-02T15:04:05Z"), Basis: it.Basis}
	return writeRecommendations(st.WS, recs)
}

// dropRecommendation removes the row of a decided item.
func dropRecommendation(ws *load.Workspace, id string) error {
	recs, err := readRecommendations(ws)
	if err != nil || len(recs) == 0 {
		return err
	}
	if _, ok := recs[id]; !ok {
		return nil
	}
	delete(recs, id)
	return writeRecommendations(ws, recs)
}

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
