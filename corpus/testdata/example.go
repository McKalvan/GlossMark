// Package horsefactory assembles horses.
//
// @gloss implements impl:HorseFactoryService scope=file
package horsefactory

// @gloss implements impl:HoofStep
func (b *HorseBuilder) AttachHoof(h Hoof) error { return nil }

// @gloss represents zoo:Horse
type Horse struct {
	ID string
}
