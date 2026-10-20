// Command notifier tells policyholders when their claim payout has been authorized.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"gopkg.in/yaml.v3"

	"github.com/tidewater/claims/payout-notifier/internal/consumer"
	"github.com/tidewater/claims/payout-notifier/internal/notify"
)

func main() {
	cfgPath := flag.String("config", "config/consumer-group.yaml", "consumer group configuration")
	flag.Parse()

	raw, err := os.ReadFile(*cfgPath)
	if err != nil {
		slog.Error("read config", "err", err)
		os.Exit(1)
	}
	var cfg consumer.Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		slog.Error("parse config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	aws, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		slog.Error("aws config", "err", err)
		os.Exit(1)
	}
	n := &notify.Notifier{
		From:      os.Getenv("NOTIFY_FROM"),
		Directory: notify.NewPolicyAdminDirectory(os.Getenv("POLICY_ADMIN_URL")),
		Sender:    sesv2.NewFromConfig(aws),
	}
	dec, err := consumer.NewAvroDecoder(os.Getenv("SCHEMA_REGISTRY_URL"))
	if err != nil {
		slog.Error("schema registry", "err", err)
		os.Exit(1)
	}
	c, err := consumer.New(cfg, os.Getenv("KAFKA_BOOTSTRAP"), dec, n)
	if err != nil {
		slog.Error("kafka", "err", err)
		os.Exit(1)
	}
	if err := c.Run(ctx); err != nil {
		slog.Error("consumer stopped", "err", err)
		os.Exit(1)
	}
}
