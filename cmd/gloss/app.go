// @gloss implements gmi:PullRequestApp scope=file

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mckalvan/glossmark/internal/prapp"
)

func init() {
	register(command{"app", "gloss app serve: the hosted GitHub App that gates pull requests with no CI setup: it receives the webhooks, checks the PR out, runs gloss diff and posts the impact comment, a check run and a review request to the owners of the HARD changes", cmdApp})
}

func cmdApp(args []string) int {
	fs, _ := newFlags("app")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) != "serve" {
		fmt.Fprintln(os.Stderr, "usage: gloss app serve  (APP_ID, PRIVATE_KEY or PRIVATE_KEY_FILE, WEBHOOK_SECRET, LISTEN_ADDR, WORKDIR; docs/APP.md)")
		return 2
	}
	cfg, err := prapp.FromEnv()
	if err != nil {
		return fail(err)
	}
	key, err := prapp.ParsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return fail(err)
	}
	srv := prapp.NewServer(prapp.NewClient(cfg.API, cfg.AppID, key), appRunner, cfg.Workdir, nil)
	mux := http.NewServeMux()
	mux.Handle("/webhook", srv.Handler(cfg.WebhookSecret))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintln(w, "ok") })
	hs := &http.Server{Addr: cfg.ListenAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(shutdown)
	}()
	fmt.Fprintf(os.Stderr, "gloss app: listening on %s (POST /webhook, GET /healthz) as App %s\n", cfg.ListenAddr, cfg.AppID)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	// Finish the pull requests in flight before the process ends.
	srv.Wait()
	return 0
}

// appRunner is the gate as gloss diff runs it, in process, on a checkout
// the app made: gloss build (the pipeline) then the classification
// against base, as the PR author, rendered as the PR comment.
func appRunner(ctx context.Context, dir, base, actor string) (*prapp.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d, err := runDiff(dir, base, actor, false)
	if err != nil {
		return nil, err
	}
	return &prapp.Result{Impact: d.im, Markdown: d.markdown(3), WS: d.head.WS}, nil
}
