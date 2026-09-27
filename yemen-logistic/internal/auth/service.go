// Package auth issues and verifies the HS256 session JWTs used by the HTTP
// API, and hashes/verifies merchant passwords with bcrypt. This is
// deliberately separate from the checkpoint package's RS256 QR-token signer:
// session tokens are only ever checked by our own server, so a symmetric
// secret is appropriate and operationally simpler than a keypair.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid username or password")
	ErrInvalidToken       = errors.New("auth: invalid or expired token")
)

type contextKey string

const claimsContextKey contextKey = "auth.claims"

// Claims is the payload embedded in every session JWT this service issues.
type Claims struct {
	MerchantID string `json:"sub"`
	Role       string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	secret   []byte
	tokenTTL time.Duration
}

// NewService constructs an auth Service. secret MUST come from an
// environment variable in every environment -- never hardcode it in source,
// or anyone with repository read access could forge valid merchant sessions.
func NewService(secret string, tokenTTL time.Duration) (*Service, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("auth: JWT secret must be at least 32 bytes, got %d", len(secret))
	}
	return &Service{secret: []byte(secret), tokenTTL: tokenTTL}, nil
}

func (s *Service) HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

func (s *Service) VerifyPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}

func (s *Service) IssueToken(merchantID, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		MerchantID: merchantID,
		Role:       role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
			Subject:   merchantID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("signing session token: %w", err)
	}
	return signed, nil
}

// ParseToken verifies signature, expiry, and algorithm, explicitly pinning
// HS256 to prevent algorithm-confusion attacks (e.g. a forged "none" token
// being accepted).
func (s *Service) ParseToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// Middleware validates the Bearer token on every protected request and
// injects the parsed Claims into the request context.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			writeAuthError(w, http.StatusUnauthorized, "missing_token")
			return
		}

		claims, err := s.ParseToken(parts[1])
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "invalid_token")
			return
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ClaimsFromContext retrieves the Claims injected by Middleware.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(*Claims)
	return claims, ok
}

func writeAuthError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, code)
}