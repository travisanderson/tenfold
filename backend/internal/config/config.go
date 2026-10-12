// Package config reads the server's settings from the environment.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds every setting the server needs. See .env.example.
type Config struct {
	Port        string
	DatabaseURL string

	// Supabase: the database host, the auth provider and the file store.
	SupabaseURL        string
	SupabaseServiceKey string
	StorageBucket      string

	// JWKSURL is where Supabase publishes its token signing keys. JWTSecret
	// is only for older projects that sign with a shared secret.
	JWKSURL   string
	JWTSecret string

	// The demo logs live in SamplesBucket. SamplesDir, when set, reads
	// them from a folder on disk instead, for working offline.
	SamplesBucket string
	SamplesDir    string

	// DBMaxConns caps the connection pool. Supabase's session pooler
	// allows a small, fixed number of connections per database, so the
	// pool must stay well under it.
	DBMaxConns int

	// AIProvider picks the decision and narrative provider: "claude",
	// "mock" or "none". Claude needs AnthropicAPIKey. The two models are
	// optional; empty means the pipeline's defaults.
	AIProvider      string
	AnthropicAPIKey string
	JudgeModel      string
	NarrativeModel  string

	RiskThreshold int
}

// Load reads the environment and reports every problem at once.
func Load() (Config, error) {
	c := Config{
		Port:               env("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		SupabaseURL:        strings.TrimRight(os.Getenv("SUPABASE_URL"), "/"),
		SupabaseServiceKey: os.Getenv("SUPABASE_SERVICE_KEY"),
		StorageBucket:      env("STORAGE_BUCKET", "logs"),
		JWKSURL:            os.Getenv("SUPABASE_JWKS_URL"),
		JWTSecret:          os.Getenv("SUPABASE_JWT_SECRET"),
		SamplesBucket:      env("SAMPLES_BUCKET", "samples"),
		SamplesDir:         os.Getenv("SAMPLES_DIR"),
		AIProvider:         env("AI_PROVIDER", "mock"),
		AnthropicAPIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		JudgeModel:         os.Getenv("AI_JUDGE_MODEL"),
		NarrativeModel:     os.Getenv("AI_NARRATIVE_MODEL"),
	}

	var problems []string
	for name, value := range map[string]string{
		"DATABASE_URL":         c.DatabaseURL,
		"SUPABASE_URL":         c.SupabaseURL,
		"SUPABASE_SERVICE_KEY": c.SupabaseServiceKey,
	} {
		if value == "" {
			problems = append(problems, name+" is required")
		}
	}
	if c.JWKSURL == "" && c.JWTSecret == "" && c.SupabaseURL != "" {
		c.JWKSURL = c.SupabaseURL + "/auth/v1/.well-known/jwks.json"
	}
	switch c.AIProvider {
	case "mock", "none":
	case "claude":
		if c.AnthropicAPIKey == "" {
			problems = append(problems, "ANTHROPIC_API_KEY is required when AI_PROVIDER is claude")
		}
	default:
		problems = append(problems, fmt.Sprintf("AI_PROVIDER %q is not claude, mock or none", c.AIProvider))
	}
	threshold, err := strconv.Atoi(env("RISK_THRESHOLD", "30"))
	if err != nil || threshold < 0 || threshold > 100 {
		problems = append(problems, "RISK_THRESHOLD must be an integer from 0 to 100")
	}
	c.RiskThreshold = threshold

	c.DBMaxConns, err = strconv.Atoi(env("DB_MAX_CONNS", "8"))
	if err != nil || c.DBMaxConns < 2 {
		problems = append(problems, "DB_MAX_CONNS must be an integer of at least 2")
	}

	if len(problems) > 0 {
		return Config{}, errors.New(strings.Join(problems, "; "))
	}
	return c, nil
}

// LoadDotEnv sets variables from a KEY=VALUE file, if the file exists.
// Variables already set in the environment win, so a deploy's real settings
// are never overridden by a stray file.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
