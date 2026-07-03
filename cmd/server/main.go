package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/eslam/cms/internal/audit"
	"github.com/eslam/cms/internal/auth"
	"github.com/eslam/cms/internal/cache"
	"github.com/eslam/cms/internal/config"
	"github.com/eslam/cms/internal/db"
	"github.com/eslam/cms/internal/httpapi"
	"github.com/eslam/cms/internal/publish"
	"github.com/eslam/cms/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	var c cache.Cache
	if cfg.RedisAddr != "" {
		c, err = cache.NewRedis(cfg.RedisAddr)
		if err != nil {
			log.Fatalf("redis (%s): %v", cfg.RedisAddr, err)
		}
		log.Printf("cache: redis at %s", cfg.RedisAddr)
	} else {
		c = cache.NewMemory()
		log.Printf("cache: in-memory (set REDIS_ADDR for redis)")
	}

	var store storage.Storage
	if cfg.S3Bucket != "" {
		store, err = storage.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket,
			cfg.AWSAccessKey, cfg.AWSSecretKey)
		if err != nil {
			log.Fatalf("s3 storage: %v", err)
		}
		log.Printf("storage: s3 bucket %q", cfg.S3Bucket)
	} else {
		store, err = storage.NewLocal(filepath.Join(cfg.DataDir, "media"), cfg.PublicBaseURL+"/media")
		if err != nil {
			log.Fatalf("local storage: %v", err)
		}
		log.Printf("storage: local disk (set S3_BUCKET for s3)")
	}

	if cfg.GitHubToken != "" && cfg.GitHubOwner != "" {
		log.Printf("publishing: GitHub push enabled (owner %s)", cfg.GitHubOwner)
	} else {
		log.Printf("publishing: local only (set GITHUB_TOKEN and GITHUB_OWNER to push to GitHub)")
	}

	auditLogger := audit.New(pool)
	authManager := auth.NewManager(cfg.JWTSecret)
	publisher := publish.New(pool, c, cfg, auditLogger)

	if err := bootstrapAdmin(pool, cfg); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}
	{
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := db.BackfillSectionTypes(ctx, pool)
		cancel()
		if err != nil {
			log.Fatalf("backfill block types: %v", err)
		}
	}

	server := httpapi.New(pool, c, cfg, authManager, auditLogger, store, publisher, "web/admin")

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("CMS listening on %s (admin dashboard: %s/admin/)", httpServer.Addr, cfg.PublicBaseURL)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

// bootstrapAdmin creates the initial platform admin on an empty user table.
func bootstrapAdmin(pool *pgxpool.Pool, cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	password := cfg.AdminPassword
	generated := false
	if password == "" {
		var err error
		password, err = auth.RandomPassword()
		if err != nil {
			return err
		}
		generated = true
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (name, email, password_hash, is_platform_admin)
		VALUES ('Platform Admin', $1, $2, true)`, cfg.AdminEmail, hash)
	if err != nil {
		return err
	}
	if generated {
		log.Printf("created platform admin %s with password: %s (change it after first login)",
			cfg.AdminEmail, password)
	} else {
		log.Printf("created platform admin %s", cfg.AdminEmail)
	}
	return nil
}
