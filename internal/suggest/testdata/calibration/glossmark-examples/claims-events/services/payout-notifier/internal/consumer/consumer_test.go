package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/tidewater/claims/payout-notifier/internal/payout"
)

type fakeDecoder struct {
	record string
	fields map[string]any
}

func (d fakeDecoder) Decode(string, []byte) (string, map[string]any, error) {
	return d.record, d.fields, nil
}

type recordingNotifier struct{ got []payout.Payout }

func (n *recordingNotifier) PayoutAuthorized(_ context.Context, p payout.Payout) error {
	n.got = append(n.got, p)
	return nil
}

func authorized() map[string]any {
	return map[string]any{
		"payoutId":          "p-1",
		"claim":             map[string]any{"claimId": "c-1", "policyNumber": "HOM-55120", "policyholderId": "h-2"},
		"amount":            map[string]any{"amountMinor": int64(103500), "currency": "EUR"},
		"payeeIbanLast4":    "4410",
		"expectedValueDate": int32(20730),
	}
}

// @gloss verifies claim:PayoutNotification conf=0.85
func TestPayoutAuthorizedNotifiesThePolicyholder(t *testing.T) {
	n := &recordingNotifier{}
	c := &Consumer{decoder: fakeDecoder{"com.tidewater.claims.events.PayoutAuthorized", authorized()}, notifier: n}
	if err := c.Handle(context.Background(), "payouts.v1", nil); err != nil {
		t.Fatal(err)
	}
	if len(n.got) != 1 || n.got[0].FormatAmount() != "EUR 1,035.00" {
		t.Fatalf("got %+v", n.got)
	}
}

func TestUnknownRecordIsAnError(t *testing.T) {
	c := &Consumer{decoder: fakeDecoder{"com.tidewater.claims.events.Nope", nil}, notifier: &recordingNotifier{}}
	if err := c.Handle(context.Background(), "payouts.v1", nil); !errors.Is(err, ErrUnknownRecord) {
		t.Fatalf("want ErrUnknownRecord, got %v", err)
	}
}
