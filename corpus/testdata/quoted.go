// Package quoted shows that a comment may talk about the `@gloss` grammar.
package quoted

// Parse reads one `@gloss implements impl:Decoy` line; the quoted span is prose.
// @gloss implements impl:HoofStep
func Parse(s string) {}

// Flip rewrites `@gloss?` to `@gloss` on the marker's line.
func Flip() {}
