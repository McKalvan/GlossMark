// Package viewer renders the static HTML viewer: one self-contained page with
// every view's projection embedded, no server, no external assets.
package viewer

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/mckalvan/glossmark/internal/views"
)

//go:embed assets/viewer.html
var assets embed.FS

// Data is what the page receives.
type Data struct {
	Generated string              `json:"generated"`
	Repo      string              `json:"repo"`
	Views     []*views.Projection `json:"views"`
	// Diff is the optional impact report from gloss diff --format json.
	Diff json.RawMessage `json:"diff,omitempty"`
}

const placeholder = "__GLOSS_DATA__"

// Render produces the HTML page with the data embedded.
func Render(d Data) ([]byte, error) {
	tpl, err := assets.ReadFile("assets/viewer.html")
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(tpl, []byte(placeholder)) {
		return nil, fmt.Errorf("viewer template lacks %s placeholder", placeholder)
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	// Keep the JSON safe inside a <script> element.
	b = bytes.ReplaceAll(b, []byte("</"), []byte(`<\/`))
	b = bytes.ReplaceAll(b, []byte("<!--"), []byte(`<\!--`))
	return bytes.Replace(tpl, []byte(placeholder), b, 1), nil
}
