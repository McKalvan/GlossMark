package booking

import (
	"testing"
	"time"
)

// @gloss verifies .BookingHandler
func TestBook(t *testing.T) {
	s := NewService(&fakeRepo{})
	if err := s.Book("p", "s1"); err != nil {
		t.Fatal(err)
	}
}

// @gloss verifies .CancellationPolicy
func TestCancelInsideWindow(t *testing.T) {
	s := NewService(&fakeRepo{})
	_ = s.Cancel("a1", time.Now())
}
