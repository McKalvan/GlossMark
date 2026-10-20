package outbox

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/larkspur-clinics/booking-api/internal/store"
)

// sendSQS posts one message with the SQS query API; IAM auth comes from the pod's role via the sidecar proxy.
func sendSQS(ctx context.Context, queueURL string, msg store.OutboxMessage) error {
	form := url.Values{
		"Action":                               {"SendMessage"},
		"MessageBody":                          {string(msg.Payload)},
		"MessageAttribute.1.Name":              {"kind"},
		"MessageAttribute.1.Value.DataType":    {"String"},
		"MessageAttribute.1.Value.StringValue": {msg.Kind},
		"MessageDeduplicationId":               {msg.ID.String()},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, queueURL+"?"+form.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sqs send: %s", resp.Status)
	}
	return nil
}
