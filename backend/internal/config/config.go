// Package config reads the server's settings from the environment.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"slices"
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
	// SupabasePublicURL is the project URL as the browser reaches it. It
	// is SupabaseURL unless set. The two differ when the server runs in
	// Docker and the browser does not.
	SupabasePublicURL string
	StorageBucket     string

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

	// Who fills each AI role. Each is "jev", "claude", "mock" or "none",
	// except that Jev cannot narrate.
	//
	//	JudgeProvider        labels each anomaly; its label can set a group aside
	//	ShadowJudgeProvider  labels the same anomalies for comparison only
	//	NarrativeProvider    writes each incident's account
	JudgeProvider       string
	ShadowJudgeProvider string
	NarrativeProvider   string

	// A key is required only when a role names its provider. The models
	// are optional; empty means the pipeline's defaults.
	AnthropicAPIKey      string
	JevAPIKey            string
	JevModel             string
	ClaudeJudgeModel     string
	ClaudeNarrativeModel string

	RiskThreshold int
}

// Load reads the environment and reports every problem at once.
func Load() (Config, error) {
	c := Config{
		Port:                 env("PORT", "8080"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		SupabaseURL:          strings.TrimRight(os.Getenv("SUPABASE_URL"), "/"),
		SupabaseServiceKey:   os.Getenv("SUPABASE_SERVICE_KEY"),
		SupabasePublicURL:    strings.TrimRight(os.Getenv("SUPABASE_PUBLIC_URL"), "/"),
		StorageBucket:        env("STORAGE_BUCKET", "logs"),
		JWKSURL:              os.Getenv("SUPABASE_JWKS_URL"),
		JWTSecret:            os.Getenv("SUPABASE_JWT_SECRET"),
		SamplesBucket:        env("SAMPLES_BUCKET", "samples"),
		SamplesDir:           os.Getenv("SAMPLES_DIR"),
		JudgeProvider:        env("AI_JUDGE_PROVIDER", "mock"),
		ShadowJudgeProvider:  env("AI_SHADOW_JUDGE_PROVIDER", "none"),
		NarrativeProvider:    env("AI_NARRATIVE_PROVIDER", "mock"),
		AnthropicAPIKey:      os.Getenv("ANTHROPIC_API_KEY"),
		JevAPIKey:            os.Getenv("JEV_API_KEY"),
		JevModel:             os.Getenv("JEV_MODEL"),
		ClaudeJudgeModel:     os.Getenv("CLAUDE_JUDGE_MODEL"),
		ClaudeNarrativeModel: os.Getenv("CLAUDE_NARRATIVE_MODEL"),
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
	if c.SupabasePublicURL == "" {
		c.SupabasePublicURL = c.SupabaseURL
	}
	if c.JWKSURL == "" && c.JWTSecret == "" && c.SupabaseURL != "" {
		c.JWKSURL = c.SupabaseURL + "/auth/v1/.well-known/jwks.json"
	}
	problems = append(problems, c.aiProblems()...)
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

// aiProblems checks the three AI roles and the keys they need.
func (c Config) aiProblems() []string {
	var problems []string
	if os.Getenv("AI_PROVIDER") != "" {
		problems = append(problems,
			"AI_PROVIDER is no longer read; set AI_JUDGE_PROVIDER, AI_SHADOW_JUDGE_PROVIDER and AI_NARRATIVE_PROVIDER")
	}
	roles := []struct{ name, value, allowed string }{
		{"AI_JUDGE_PROVIDER", c.JudgeProvider, "jev claude mock none"},
		{"AI_SHADOW_JUDGE_PROVIDER", c.ShadowJudgeProvider, "jev claude mock none"},
		{"AI_NARRATIVE_PROVIDER", c.NarrativeProvider, "claude mock none"}, // Jev does not write text
	}
	for _, role := range roles {
		if !slices.Contains(strings.Fields(role.allowed), role.value) {
			problems = append(problems, fmt.Sprintf("%s %q is not one of: %s", role.name, role.value, role.allowed))
		}
		if role.value == "claude" && c.AnthropicAPIKey == "" {
			problems = append(problems, fmt.Sprintf("ANTHROPIC_API_KEY is required when %s is claude", role.name))
		}
		if role.value == "jev" && c.JevAPIKey == "" {
			problems = append(problems, fmt.Sprintf("JEV_API_KEY is required when %s is jev", role.name))
		}
	}
	if c.ShadowJudgeProvider != "none" && c.ShadowJudgeProvider == c.JudgeProvider {
		problems = append(problems, "AI_SHADOW_JUDGE_PROVIDER must differ from AI_JUDGE_PROVIDER; a judge compared with itself says nothing")
	}
	return problems
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
