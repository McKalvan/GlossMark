// Package notify tells a policyholder about their payout by email.
package notify

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/tidewater/claims/payout-notifier/internal/payout"
)

// Directory resolves a policyholder id to the address they asked to be contacted at.
type Directory interface {
	EmailFor(ctx context.Context, policyholderID string) (string, error)
}

// Sender sends one email.
type Sender interface {
	SendEmail(ctx context.Context, in *sesv2.SendEmailInput, opts ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error)
}

// Notifier renders and sends payout notifications.
type Notifier struct {
	From      string
	Directory Directory
	Sender    Sender
}

type view struct {
	PolicyNumber, Amount, IBANLast4, ValueDate string
}

// PayoutAuthorized emails the policyholder that their payout has been authorized.
// @gloss implements .PayoutEmail conf=0.9
func (n *Notifier) PayoutAuthorized(ctx context.Context, p payout.Payout) error {
	to, err := n.Directory.EmailFor(ctx, p.PolicyholderID)
	if err != nil {
		return fmt.Errorf("resolve contact for %s: %w", p.PolicyholderID, err)
	}
	v := view{p.PolicyNumber, p.FormatAmount(), p.PayeeIBANLast4, p.ExpectedValueDate.Format("2 January 2006")}
	var subject, body bytes.Buffer
	if err := payoutAuthorizedSubject.Execute(&subject, v); err != nil {
		return err
	}
	if err := payoutAuthorizedBody.Execute(&body, v); err != nil {
		return err
	}
	_, err = n.Sender.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(n.From),
		Destination:      &types.Destination{ToAddresses: []string{to}},
		Content: &types.EmailContent{Simple: &types.Message{
			Subject: &types.Content{Data: aws.String(subject.String())},
			Body:    &types.Body{Text: &types.Content{Data: aws.String(body.String())}},
		}},
	})
	return err
}
