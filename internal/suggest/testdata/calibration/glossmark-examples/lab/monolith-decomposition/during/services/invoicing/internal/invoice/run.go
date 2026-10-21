package invoice

import (
	"context"
	"errors"
	"time"
)

const vatBasisPoints = 2000

type Store interface {
	Insert(ctx context.Context, inv *Invoice) error
}

type Request struct {
	HireID        int64
	VATRegistered bool
	DayRatePence  int64
	StartsOn      time.Time
	ReturnedOn    time.Time
}

type Service struct {
	Store Store
	Now   func() time.Time
}

var ErrNotReturned = errors.New("invoice: hire has not been returned")

// @gloss implements .InvoiceRun
func (s *Service) Issue(ctx context.Context, req Request) (*Invoice, error) {
	if req.ReturnedOn.IsZero() {
		return nil, ErrNotReturned
	}
	days := int64(req.ReturnedOn.Sub(req.StartsOn).Hours()/24) + 1
	net := req.DayRatePence * days
	var vat int64
	if req.VATRegistered {
		vat = (net*vatBasisPoints + 5000) / 10000
	}
	inv := &Invoice{HireID: req.HireID, NetPence: net, VATPence: vat, IssuedAt: s.Now()}
	if err := s.Store.Insert(ctx, inv); err != nil {
		return nil, err
	}
	return inv, nil
}
