// Package consumer reads payouts.v1 and hands each payout event to the notifier.
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/tidewater/claims/payout-notifier/internal/payout"
)

// ErrUnknownRecord is returned for a record type the notifier does not know.
var ErrUnknownRecord = errors.New("unknown record type")

// Decoder turns a raw Avro value into its record name and a generic map.
type Decoder interface {
	Decode(topic string, value []byte) (record string, fields map[string]any, err error)
}

// Notifier is what the consumer calls for each payout event it handles.
type Notifier interface {
	PayoutAuthorized(ctx context.Context, p payout.Payout) error
}

// Config is the consumer-group configuration, loaded from config/consumer-group.yaml.
type Config struct {
	Topic    string            `yaml:"topic"`
	Settings map[string]string `yaml:"consumer"`
}

// Consumer is the payout-notifier's single consumer group member.
type Consumer struct {
	kafka    *kafka.Consumer
	decoder  Decoder
	notifier Notifier
	topic    string
}

// New builds a consumer from the group configuration.
func New(cfg Config, bootstrap string, decoder Decoder, notifier Notifier) (*Consumer, error) {
	cm := kafka.ConfigMap{"bootstrap.servers": bootstrap}
	for k, v := range cfg.Settings {
		cm[k] = v
	}
	kc, err := kafka.NewConsumer(&cm)
	if err != nil {
		return nil, err
	}
	return &Consumer{kafka: kc, decoder: decoder, notifier: notifier, topic: cfg.Topic}, nil
}

// Run polls until ctx is cancelled. Offsets are committed only after a message is handled.
func (c *Consumer) Run(ctx context.Context) error {
	if err := c.kafka.SubscribeTopics([]string{c.topic}, nil); err != nil {
		return err
	}
	defer c.kafka.Close()
	for ctx.Err() == nil {
		msg, err := c.kafka.ReadMessage(500 * time.Millisecond)
		if err != nil {
			var kerr kafka.Error
			if errors.As(err, &kerr) && kerr.IsTimeout() {
				continue
			}
			return err
		}
		if err := c.Handle(ctx, *msg.TopicPartition.Topic, msg.Value); err != nil {
			slog.Error("payout notification failed", "offset", msg.TopicPartition.Offset, "err", err)
			continue
		}
		if _, err := c.kafka.CommitMessage(msg); err != nil {
			return err
		}
	}
	return nil
}

// Handle decodes one message and notifies the policyholder.
// @gloss subscribes claim:PayoutAuthorized conf=0.85
// @gloss consumes claim:Payout conf=0.85
func (c *Consumer) Handle(ctx context.Context, topic string, value []byte) error {
	record, f, err := c.decoder.Decode(topic, value)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	switch record {
	case "com.tidewater.claims.events.PayoutAuthorized":
		return c.notifier.PayoutAuthorized(ctx, toPayout(f))
	case "com.tidewater.claims.events.PayoutFailed":
		// TODO(PAY-412): tell the policyholder their payment bounced and ask for new
		// bank details. Until then Treasury phones them; skip without failing.
		slog.Info("payout failed; customer notification not implemented", "claim", claimID(f))
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnknownRecord, record)
	}
}

func toPayout(f map[string]any) payout.Payout {
	ref := f["claim"].(map[string]any)
	amount := f["amount"].(map[string]any)
	return payout.Payout{
		ID:                f["payoutId"].(string),
		ClaimID:           ref["claimId"].(string),
		PolicyNumber:      ref["policyNumber"].(string),
		PolicyholderID:    ref["policyholderId"].(string),
		AmountMinor:       amount["amountMinor"].(int64),
		Currency:          amount["currency"].(string),
		PayeeIBANLast4:    f["payeeIbanLast4"].(string),
		ExpectedValueDate: time.Unix(0, 0).UTC().AddDate(0, 0, int(f["expectedValueDate"].(int32))),
	}
}

func claimID(f map[string]any) string {
	if ref, ok := f["claim"].(map[string]any); ok {
		if id, ok := ref["claimId"].(string); ok {
			return id
		}
	}
	return ""
}
