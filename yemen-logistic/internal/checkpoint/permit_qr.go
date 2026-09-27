package checkpoint

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"yemen-mit-platform/internal/domain"
)


//  A permit QR code is generated and expected to be scanned
// within days of a specific truck's checkpoint transit, not held for weeks
// like a printed invoice might be  a shorter window reduces the exposure
// if a code is photographed or intercepted.
const permitTokenValidityDuration = 72 * time.Hour

// Sentinel errors specific to permit QR tokens, kept distinct from the
// invoice ones above so a caller can tell which kind of token failed if
// both code paths are ever in play in the same place.
var (
	ErrEmptyPermitToken       = errors.New("checkpoint: permit token string is empty")
	ErrPermitTokenExpired     = errors.New("checkpoint: permit QR token has expired")
	ErrPermitInvalidSignature = errors.New("checkpoint: permit QR token signature is invalid")
	ErrPermitInvalidIssuer    = errors.New("checkpoint: permit QR token issuer is not recognized")
	ErrPermitMalformedClaims  = errors.New("checkpoint: permit QR token payload is missing required claims")
)

// PermitQRClaims is the JWT payload embedded in every merchant permit's QR
// code. Unlike CustomClaims (built for invoices, which only carries IDs
// because a connected system can look the rest up), this embeds the
// trader's display name and the cargo details directly a checkpoint
// device with zero signal has nothing to look those IDs up against, so
// everything the officer needs to see has to travel inside the signed
// token itself.
type PermitQRClaims struct {
	PermitID       string  `json:"permit_id"`
	MerchantID     string  `json:"merchant_id"`
	MerchantNameAr string  `json:"merchant_name_ar"`
	MerchantNameEn string  `json:"merchant_name_en"`
	CargoType      string  `json:"cargo_type"`
	Weight         float64 `json:"weight"`
	Value          float64 `json:"value"`
	Duty           float64 `json:"duty"`

	jwt.RegisteredClaims
}

// GenerateSecurePermitQRToken signs a permit's checkpoint-relevant details,
// plus the issuing merchant's display name, into a compact RS256-signed JWT
// with a 3-day expiry. Like GenerateSecureQRToken, this must only ever run
// on the central, connected Ministry server never on a checkpoint
// device.
func GenerateSecurePermitQRToken(permit domain.Permit, merchant domain.Merchant, privateKeyHex string) (string, error) {
	if strings.TrimSpace(permit.ID) == "" {
		return "", errors.New("checkpoint: cannot generate permit QR token, permit ID is empty")
	}
	if strings.TrimSpace(permit.MerchantID) == "" {
		return "", errors.New("checkpoint: cannot generate permit QR token, permit merchant ID is empty")
	}

	privateKey, err := decodePrivateKey(privateKeyHex)
	if err != nil {
		return "", err
	}

	jti, err := generateJTI()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()

	claims := PermitQRClaims{
		PermitID:       permit.ID,
		MerchantID:     permit.MerchantID,
		MerchantNameAr: merchant.CompanyNameAr,
		MerchantNameEn: merchant.CompanyNameEn,
		CargoType:      permit.Type,
		Weight:         permit.Weight,
		Value:          permit.Value,
		Duty:           permit.Duty,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   permit.ID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(permitTokenValidityDuration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("checkpoint: failed to sign permit QR token: %w", err)
	}

	return signedToken, nil
}

// VerifyPermitQRTokenOffline validates a permit QR token using ONLY the
// supplied public key no database, no network call anywhere in this
// function. This is the Go side mirror of the verification the officer's
// browser performs locally via the Web Crypto API it exists here too so
// the same logic is unit-testable and could be re-run server-side later if
// ever needed e.g. audit reconciliation once connectivity returns
func VerifyPermitQRTokenOffline(tokenString string, publicKeyHex string) (*PermitQRClaims, error) {
	trimmedToken := strings.TrimSpace(tokenString)
	if trimmedToken == "" {
		return nil, ErrEmptyPermitToken
	}

	publicKey, err := decodePublicKey(publicKeyHex)
	if err != nil {
		return nil, err
	}

	claims := &PermitQRClaims{}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(tokenIssuer),
	)

	parsedToken, err := parser.ParseWithClaims(trimmedToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("checkpoint: unexpected signing method %q", t.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrPermitTokenExpired
		case errors.Is(err, jwt.ErrTokenSignatureInvalid):
			return nil, ErrPermitInvalidSignature
		case errors.Is(err, jwt.ErrTokenInvalidIssuer):
			return nil, ErrPermitInvalidIssuer
		default:
			return nil, fmt.Errorf("%w: %v", ErrPermitInvalidSignature, err)
		}
	}

	if !parsedToken.Valid {
		return nil, ErrPermitInvalidSignature
	}

	if strings.TrimSpace(claims.PermitID) == "" || strings.TrimSpace(claims.MerchantID) == "" {
		return nil, ErrPermitMalformedClaims
	}

	return claims, nil
}