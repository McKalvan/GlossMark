package invoice

import (
	"context"
	"testing"
	"time"
)

type memStore struct{ rows []*Invoice }

func (m *memStore) Insert(_ context.Context, inv *Invoice) error {
	m.rows = append(m.rows, inv)
	return nil
}

// @gloss verifies hire:Invoicing
func TestIssuePricesInclusiveDays(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	svc := &Service{Store: &memStore{}, Now: time.Now}
	inv, err := svc.Issue(context.Background(), Request{HireID: 7, DayRatePence: 4500, StartsOn: start, ReturnedOn: start.AddDate(0, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if inv.NetPence != 3*4500 {
		t.Fatalf("net = %d, want %d", inv.NetPence, 3*4500)
	}
}
