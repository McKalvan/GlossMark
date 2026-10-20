// Package payout holds the payout as the notifier sees it: decoded from PayoutAuthorized.
package payout

import (
	"fmt"
	"time"
)

// Payout is money cleared for payment to a policyholder for one approved claim.
// @gloss represents claim:Payout conf=0.95
type Payout struct {
	ID                string
	ClaimID           string
	PolicyNumber      string
	PolicyholderID    string
	AmountMinor       int64
	Currency          string
	PayeeIBANLast4    string
	ExpectedValueDate time.Time
}

// FormatAmount renders the amount for a customer message, e.g. "EUR 1,035.00".
func (p Payout) FormatAmount() string {
	major := p.AmountMinor / 100
	minor := p.AmountMinor % 100
	return fmt.Sprintf("%s %s.%02d", p.Currency, groupThousands(major), minor)
}

func groupThousands(n int64) string {
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
