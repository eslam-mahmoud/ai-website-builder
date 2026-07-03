package config

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all runtime configuration, loaded from environment
// variables (optionally seeded from a .env file).
type Config struct {
	Port          string
	PublicBaseURL string
	DataDir       string

	DatabaseURL string
	RedisAddr   string

	JWTSecret string

	AdminEmail    string
	AdminPassword string

	// S3-compatible storage. When S3Bucket is empty, media is stored on
	// local disk under DataDir/media.
	S3Bucket     string
	S3Region     string
	S3Endpoint   string
	AWSAccessKey string
	AWSSecretKey string

	// GitHub. When GitHubToken and GitHubOwner are set, publishing pushes
	// generated sites to one repository per website.
	GitHubToken string
	GitHubOwner string

	// Cloudflare. Used to poll Pages deployment status and purge cache.
	CloudflareAPIKey    string
	CloudflareAccountID string
}

// Load reads .env (if present) then environment variables.
func Load() *Config {
	loadDotEnv(".env")

	cfg := &Config{
		Port:          getenv("PORT", "8080"),
		PublicBaseURL: getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
		DataDir:       getenv("DATA_DIR", "var"),

		DatabaseURL: getenv("DATABASE_URL", "postgres://cms:cms@localhost:5433/cms?sslmode=disable"),
		RedisAddr:   os.Getenv("REDIS_ADDR"),

		JWTSecret: os.Getenv("JWT_SECRET"),

		AdminEmail:    getenv("ADMIN_EMAIL", "admin@cms.local"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),

		S3Bucket:     os.Getenv("S3_BUCKET"),
		S3Region:     getenv("S3_REGION", "us-east-1"),
		S3Endpoint:   getenv("S3_ENDPOINT", "s3.amazonaws.com"),
		AWSAccessKey: os.Getenv("AWS_ACCESS_KEY"),
		AWSSecretKey: os.Getenv("AWS_SECRET_KEY"),

		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		GitHubOwner: os.Getenv("GITHUB_OWNER"),

		CloudflareAPIKey:    os.Getenv("CLOUDFLARE_API_KEY"),
		CloudflareAccountID: os.Getenv("CLOUDFLARE_ACCOUNT_ID"),
	}

	if cfg.JWTSecret == "" {
		cfg.JWTSecret = persistentSecret(filepath.Join(cfg.DataDir, ".jwt_secret"))
	}
	return cfg
}

// persistentSecret loads or creates a random secret at path so JWT sessions
// survive restarts in dev without requiring JWT_SECRET to be set.
func persistentSecret(path string) string {
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		return strings.TrimSpace(string(b))
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("generate jwt secret: %v", err)
	}
	secret := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		_ = os.WriteFile(path, []byte(secret), 0o600)
	}
	return secret
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadDotEnv sets variables from a KEY=VALUE file without overriding
// values already present in the environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: reading %s: %v\n", path, err)
	}
}
