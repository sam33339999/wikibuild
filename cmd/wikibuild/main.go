// Command wikibuild starts the WikiBuilder server. It loads configuration
// from a .env file (if present) and environment variables, bootstraps the
// admin user on first run, and serves the Fiber app against PostgreSQL.
package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/sam33339999/wikibuild/internal/config"
	"github.com/sam33339999/wikibuild/internal/llm"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/scheduler"
	"github.com/sam33339999/wikibuild/internal/server"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/postgres"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("wikibuild: %v", err)
	}
}

func run() error {
	// Load .env if present. Existing environment variables take precedence
	// (godotenv.Load never overwrites them), so production deploys that set
	// real env vars are unaffected. A missing .env is not an error.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("wikibuild: warning loading .env: %v", err)
	}

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	log.Printf("wikibuild: listening on %s", addr)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Schema must be applied out-of-band (see `make migrate-up`).
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	repo := postgres.New(pool)
	if err := ensureAdmin(ctx, repo, cfg.AdminUser, cfg.AdminPass); err != nil {
		return err
	}

	clk := clock.Real{}
	llmClient := llm.NewOpenAIClient(llm.OpenAIConfig{
		BaseURL: cfg.LLMBaseURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
	})
	if llmClient.Enabled() {
		log.Printf("wikibuild: LLM SEO enabled (model=%s)", cfg.LLMModel)
	}
	app := server.New(server.Deps{
		Store:           repo,
		Hasher:          auth.NewPasswordHasher(),
		Signer:          auth.NewSigner(cfg.SessionSecret, clk),
		Limiter:         auth.NewLoginLimiter(clk, auth.DefaultLimiterConfig()),
		Clock:           clk,
		SiteDefaultPass: cfg.DefaultProtectedPass,
		ContentDir:      cfg.ContentDir,
		StaticDir:       "./static",
		BaseURL:         cfg.BaseURL,
		SiteTitle:       cfg.SiteTitle,
		LLM:             llmClient,
	})

	// Background publisher for scheduled drafts.
	pubCtx, pubCancel := context.WithCancel(context.Background())
	defer pubCancel()
	go runPublisher(pubCtx, repo, clk)

	// Graceful shutdown on SIGINT / SIGTERM.
	errCh := make(chan error, 1)
	go func() {
		if err := app.Listen(addr); err != nil {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		pubCancel()
		return err
	case <-sigCh:
		log.Println("wikibuild: shutting down")
		pubCancel()
		return app.ShutdownWithTimeout(10 * time.Second)
	}
}

// runPublisher ticks the scheduled-publish scheduler until ctx is cancelled.
func runPublisher(ctx context.Context, repo store.Repository, clk clock.Clock) {
	p := &scheduler.Publisher{Repo: repo, Clock: clk}
	// Immediate pass on boot so overdue drafts go live without waiting.
	if n, err := p.Tick(ctx); err != nil {
		log.Printf("wikibuild: publisher: %v", err)
	} else if n > 0 {
		log.Printf("wikibuild: publisher: published %d scheduled article(s)", n)
	}
	t := time.NewTicker(scheduler.DefaultInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := p.Tick(ctx); err != nil {
				log.Printf("wikibuild: publisher: %v", err)
			} else if n > 0 {
				log.Printf("wikibuild: publisher: published %d scheduled article(s)", n)
			}
		}
	}
}

// ensureAdmin creates the admin user if it does not already exist, hashing the
// initial password from config. On subsequent starts it is a no-op.
func ensureAdmin(ctx context.Context, repo store.Repository, username, password string) error {
	_, err := repo.GetUserByUsername(ctx, username)
	if err == nil {
		return nil // already exists
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	hash, err := auth.NewPasswordHasher().Hash(password)
	if err != nil {
		return err
	}
	_, err = repo.CreateUser(ctx, model.User{
		Username:     username,
		PasswordHash: hash,
		CreatedAt:    time.Now(),
	})
	return err
}
