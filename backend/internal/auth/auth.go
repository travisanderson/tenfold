// Package auth verifies Supabase access tokens and carries the caller
// through the request context.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// User is the caller, taken from a verified token.
type User struct {
	ID    uuid.UUID
	Email string
}

// Name is a short display name: the part of the email before the @.
func (u User) Name() string {
	if name, _, ok := strings.Cut(u.Email, "@"); ok && name != "" {
		return name
	}
	if u.Email != "" {
		return u.Email
	}
	return u.ID.String()[:8]
}

// Verifier checks a token's signature, expiry and audience.
type Verifier struct {
	keyfunc jwt.Keyfunc
	methods []string
}

// NewJWKSVerifier verifies tokens against the public keys Supabase
// publishes at jwksURL. The keys are fetched once and refreshed in the
// background until ctx ends.
func NewJWKSVerifier(ctx context.Context, jwksURL string) (*Verifier, error) {
	keys, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("load signing keys from %s: %w", jwksURL, err)
	}
	return &Verifier{keyfunc: keys.Keyfunc, methods: []string{"ES256", "RS256"}}, nil
}

// NewSecretVerifier verifies tokens signed with a shared HS256 secret. Older
// Supabase projects sign this way; tests and local development use it too.
func NewSecretVerifier(secret string) *Verifier {
	return &Verifier{
		keyfunc: func(*jwt.Token) (any, error) { return []byte(secret), nil },
		methods: []string{"HS256"},
	}
}

type claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// Verify returns the user a token was issued to, or an error if the token
// is not valid. Supabase puts the user's ID in the "sub" claim and marks
// signed-in users with the audience "authenticated".
func (v *Verifier) Verify(token string) (User, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, v.keyfunc,
		jwt.WithValidMethods(v.methods),
		jwt.WithAudience("authenticated"),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return User{}, err
	}
	id, err := uuid.Parse(c.Subject)
	if err != nil {
		return User{}, errors.New("token subject is not a user ID")
	}
	return User{ID: id, Email: c.Email}, nil
}

// FromHeader pulls the token out of an Authorization header value.
func FromHeader(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

type contextKey struct{}

// WithUser returns a context that carries the caller.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, contextKey{}, u)
}

// UserFrom returns the caller. It is only called on routes behind the auth
// middleware, so a missing user is a programming error.
func UserFrom(ctx context.Context) User {
	u, ok := ctx.Value(contextKey{}).(User)
	if !ok {
		panic("auth: no user in context; is the route behind the auth middleware?")
	}
	return u
}
