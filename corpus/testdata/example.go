// Package circulation lends library items.
//
// @gloss implements circ:CirculationService scope=file
package circulation

// @gloss implements circ:MarcFieldMapper
func (b *CatalogIntake) MapField(f MarcField) error { return nil }

// @gloss represents lib:Media
type Media struct {
	ID string
}
