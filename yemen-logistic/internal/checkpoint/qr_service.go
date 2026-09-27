// Package checkpoint implements the offline-first cryptographic QR
// verification service used at rural Ministry checkpoints.
//
// The core design constraint driving this entire file: an inspector at a
// checkpoint with no cellular signal and no line-of-sight to the central
// database must still be able to scan a merchant's QR code and know, with
// cryptographic certainty, that:
//
//  1. This invoice was genuinely issued by the Ministry's system (not
//     forged or edited by the merchant/driver).
//  2. The payment status, truck license, and merchant identity embedded in
//     the code have not been tampered with since issuance.
//  3. The code has not expired.
//
// This is achieved with RS256 (RSA signature with SHA-256), an asymmetric
// signing scheme: the central server holds the PRIVATE key and signs QR
// tokens at invoice-generation time; every checkpoint device is pre-loaded
// (during its last successful sync) with only the PUBLIC key, which is safe
// to distribute widely and can verify signatures without ever being able to
// forge one. This is precisely why RS256 was chosen over a symmetric scheme
// like HS256, where the same secret both signs and verifies and therefore
// cannot be safely placed on checkpoint hardware that might be lost, stolen,
// or physically tampered with in a rural setting.
package checkpoint

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"yemen-mit-platform/internal/domain"
)

const (
	// tokenIssuer identifies this system as the sole authority permitted to
	// issue QR tokens. The offline verifier rejects any token whose "iss"
	// claim does not match this exact value, even if the signature is
	// otherwise cryptographically valid  this guards against a signature
	// that was validly produced by a *different* RSA keypair the inspector's
	// device might also happen to trust in a later phase (e.g. a partner
	// customs authority's keys).
	tokenIssuer = "MIT-Yemen-Logistics-Platform"

	// tokenValidityDuration is deliberately generous (30 days) rather than
	// the minutes hours typical of a web session token. Field invoices are
	// often printed or sent over WhatsApp days before a truck physically
	// reaches a checkpoint, and connectivity to refresh a token cannot be
	// assumed. Thirty days balances operational reality against the risk
	// window of a compromised/stolen QR code.
	tokenValidityDuration = 30 * 24 * time.Hour

	// rsaKeyBits is the key size used by GenerateRSAKeyPairHex. 2048 bits is
	// the current practical minimum for RSA in production systems; it is
	// used here rather than 4096 to keep the resulting QR payload (and thus
	// the printed/scanned QR code image) small enough to reliably decode
	// from a phone camera under poor lighting and low-quality print output.
	rsaKeyBits = 2048

	// jtiByteLength is the number of random bytes used to build each token's
	// unique "jti" (JWT ID) claim, hex-encoded to a 32-character string.
	jtiByteLength = 16
)

// Sentinel errors returned by VerifyQRTokenOffline. Callers (e.g. the HTTP
// handler layer) should use errors . Is against these to decide what feedback
// to show the inspector, without needing to string-match error messages.
var (
	// ErrEmptyToken is returned when an empty or whitespace-only token
	// string is presented for verification.
	ErrEmptyToken = errors.New("checkpoint: token string is empty")

	// ErrTokenExpired is returned when the token's "exp" claim is in the
	// past relative to the checkpoint device's local clock.
	ErrTokenExpired = errors.New("checkpoint: QR token has expired")

	// ErrInvalidSignature is returned when the token's cryptographic
	// signature does not validate against the supplied public key, or the
	// token uses an unexpected/downgraded signing algorithm.
	ErrInvalidSignature = errors.New("checkpoint: QR token signature is invalid")

	// ErrInvalidIssuer is returned when the token's "iss" claim does not
	// match the expected Ministry issuer identifier.
	ErrInvalidIssuer = errors.New("checkpoint: QR token issuer is not recognized")

	// ErrMalformedClaims is returned when the token verifies
	// cryptographically but is missing required business claims — this
	// indicates either data corruption or a token that was never actually
	// produced by GenerateSecureQRToken.
	ErrMalformedClaims = errors.New("checkpoint: QR token payload is missing required claims")

	// ErrInvalidKeyEncoding is returned when a supplied key hex string
	// cannot be decoded or parsed into a usable RSA key.
	ErrInvalidKeyEncoding = errors.New("checkpoint: supplied key could not be decoded")
)

// CustomClaims is the JWT payload embedded in every invoice QR code. It
// embeds jwt.RegisteredClaims to get standard, spec-compliant handling of
// issuer, subject, expiry, not-before, issued-at, and a unique token ID —
// all of which the RS256 signature protects along with the custom business
// fields below.
type CustomClaims struct {
	// InvoiceID is the primary key of the invoice this token represents.
	InvoiceID string `json:"invoice_id"`

	// MerchantID identifies which merchant the invoice — and therefore the
	// cargo being inspected — belongs to.
	MerchantID string `json:"merchant_id"`

	// ShipmentID is included so an inspector can cross-reference physical
	// cargo markings against the shipment record once connectivity is
	// restored, without needing to look the invoice up first.
	ShipmentID string `json:"shipment_id"`

	// PaymentStatus lets an inspector immediately see, offline, whether the
	// Ministry considers this shipment's dues settled. A checkpoint policy
	// might, for instance, flag or hold any shipment whose PaymentStatus is
	// not domain.PaymentStatusPaid.
	PaymentStatus domain.PaymentStatus `json:"payment_status"`

	// TruckLicense binds the token to a specific physical vehicle, so a
	// valid QR code cannot be reused to wave through a different truck
	// carrying different (or additional) undeclared cargo.
	TruckLicense string `json:"truck_license"`

	// GrandTotal is embedded so an inspector can sanity-check declared cargo
	// value against physical cargo without needing network access to
	// recompute it from the underlying cost fields.
	GrandTotal float64 `json:"grand_total"`

	jwt.RegisteredClaims
}

// Key decoding helpers


// decodePrivateKey turns a hex-encoded DER byte string into an *rsa.PrivateKey.
// It transparently supports both PKCS#1 ("RSA PRIVATE KEY") and PKCS#8
// ("PRIVATE KEY") DER encodings, since different key-generation tooling
// (openssl genrsa vs. openssl genpkey) defaults to different formats, and

// this service should not force operators into one specific toolchain.

func decodePrivateKey(privateKeyHex string) (*rsa.PrivateKey, error) {
	trimmed := strings.TrimSpace(privateKeyHex)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: private key hex string is empty", ErrInvalidKeyEncoding)
	}

	der, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: private key is not valid hex: %v", ErrInvalidKeyEncoding, err)
	}

	if key, pkcs1Err := x509.ParsePKCS1PrivateKey(der); pkcs1Err == nil {
		return key, nil
	}

	parsedAny, pkcs8Err := x509.ParsePKCS8PrivateKey(der)
	if pkcs8Err != nil {
		return nil, fmt.Errorf(
			"%w: private key is neither valid PKCS#1 nor PKCS#8 DER: %v",
			ErrInvalidKeyEncoding, pkcs8Err,
		)
	}

	rsaKey, ok := parsedAny.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: decoded key is not an RSA private key", ErrInvalidKeyEncoding)
	}
	return rsaKey, nil
}

// decodePublicKey turns a hex-encoded PKIX/SubjectPublicKeyInfo DER byte
// string into an *rsa.PublicKey.
func decodePublicKey(publicKeyHex string) (*rsa.PublicKey, error) {
	trimmed := strings.TrimSpace(publicKeyHex)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: public key hex string is empty", ErrInvalidKeyEncoding)
	}

	der, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: public key is not valid hex: %v", ErrInvalidKeyEncoding, err)
	}

	parsedAny, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("%w: public key is not valid PKIX DER: %v", ErrInvalidKeyEncoding, err)
	}

	rsaKey, ok := parsedAny.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: decoded key is not an RSA public key", ErrInvalidKeyEncoding)
	}
	return rsaKey, nil
}

// generateJTI produces a cryptographically random, hex-encoded unique
// identifier for a token's "jti" claim. Using crypto/rand (not math/rand)
// is required here: this value contributes to preventing token replay
// tracking and must not be predictable.
func generateJTI() (string, error) {
	buf := make([]byte, jtiByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("checkpoint: failed to generate random token id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// -----------------------------------------------------------------------------
// GenerateRSAKeyPairHex
// -----------------------------------------------------------------------------

// GenerateRSAKeyPairHex generates a fresh 2048-bit RSA key pair and returns
// both keys as hex-encoded DER strings (private key in PKCS#1 form, public
// key in PKIX form), ready to be passed directly into GenerateSecureQRToken
// and VerifyQRTokenOffline respectively.
//
// In production, this function should be run exactly ONCE per deployment
// (or per key-rotation cycle) by a Ministry system administrator, with the
// resulting private key hex stored in a secrets manager / environment
// variable never checked into source control, and the public key hex
// distributed to checkpoint devices during their provisioning/sync process.
// It is exposed here as a reusable utility so `cmd/api/main.go` can also use
// it to bootstrap a working demo without requiring an external key file on
// first run.
func GenerateRSAKeyPairHex() (privateKeyHex string, publicKeyHex string, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return "", "", fmt.Errorf("checkpoint: failed to generate RSA key pair: %w", err)
	}

	privateDER := x509.MarshalPKCS1PrivateKey(privateKey)

	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("checkpoint: failed to marshal RSA public key: %w", err)
	}

	return hex.EncodeToString(privateDER), hex.EncodeToString(publicDER), nil
}

// -----------------------------------------------------------------------------
// GenerateSecureQRToken
// -----------------------------------------------------------------------------

// GenerateSecureQRToken signs the critical, checkpoint-relevant fields of an
// invoice (InvoiceID, MerchantID, ShipmentID, PaymentStatus, TruckLicense,
// GrandTotal) into a compact RS256-signed JWT suitable for encoding directly
// into a QR code image.
//
// privateKeyHex must be a hex-encoded PKCS#1 or PKCS#8 DER RSA private key,
// as produced by GenerateRSAKeyPairHex. This function is intended to run
// ONLY on the central, connected Ministry server at invoice-issuance time —
// never on a checkpoint device.
func GenerateSecureQRToken(invoice domain.Invoice, privateKeyHex string) (string, error) {
	if strings.TrimSpace(invoice.ID) == "" {
		return "", errors.New("checkpoint: cannot generate QR token, invoice ID is empty")
	}
	if strings.TrimSpace(invoice.MerchantID) == "" {
		return "", errors.New("checkpoint: cannot generate QR token, invoice merchant ID is empty")
	}
	if strings.TrimSpace(invoice.ShipmentID) == "" {
		return "", errors.New("checkpoint: cannot generate QR token, invoice shipment ID is empty")
	}
	if !invoice.PaymentStatus.IsValid() {
		return "", fmt.Errorf("checkpoint: cannot generate QR token, invalid payment status %q", invoice.PaymentStatus)
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

	claims := CustomClaims{
		InvoiceID:     invoice.ID,
		MerchantID:    invoice.MerchantID,
		ShipmentID:    invoice.ShipmentID,
		PaymentStatus: invoice.PaymentStatus,
		TruckLicense:  invoice.TruckLicense,
		GrandTotal:    invoice.GrandTotal(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   invoice.ID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenValidityDuration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("checkpoint: failed to sign QR token: %w", err)
	}

	return signedToken, nil
}

// -----------------------------------------------------------------------------
// VerifyQRTokenOffline
// -----------------------------------------------------------------------------

// VerifyQRTokenOffline fully decodes, cryptographically validates, and
// business-validates a QR token string using ONLY the supplied public key —
// no database connection, no network call, and no external service is
// touched anywhere in this function. This is precisely what makes it safe
// to run on a checkpoint device that has been completely offline since its
// last sync.
//
// publicKeyHex must be a hex-encoded PKIX DER RSA public key, as produced by
// GenerateRSAKeyPairHex or derived from the matching private key.
//
// On success, it returns the fully populated *CustomClaims extracted from
// the token. On failure, it returns one of the sentinel errors declared
// above (or a wrapped variant of one), which callers should check with
// errors.Is.
func VerifyQRTokenOffline(tokenString string, publicKeyHex string) (*CustomClaims, error) {
	trimmedToken := strings.TrimSpace(tokenString)
	if trimmedToken == "" {
		return nil, ErrEmptyToken
	}

	publicKey, err := decodePublicKey(publicKeyHex)
	if err != nil {
		return nil, err
	}

	claims := &CustomClaims{}

	parser := jwt.NewParser(
		// Explicitly pin the accepted algorithm to RS256. Without this, a
		// malicious token crafted with alg:"none" or a downgraded HMAC
		// algorithm could otherwise slip past a naively configured parser
		// (the well-known "algorithm confusion" JWT attack class).
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
			return nil, ErrTokenExpired
		case errors.Is(err, jwt.ErrTokenSignatureInvalid):
			return nil, ErrInvalidSignature
		case errors.Is(err, jwt.ErrTokenInvalidIssuer):
			return nil, ErrInvalidIssuer
		default:
			return nil, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}
	}

	if !parsedToken.Valid {
		return nil, ErrInvalidSignature
	}

	// Defense in depth: even though the signature has verified, confirm the
	// business-critical claims are actually present and non-empty. A token
	// signed correctly but missing these fields could not have come from
	// GenerateSecureQRToken and indicates either a bug upstream or a
	// deliberately crafted minimal payload — either way it must be rejected.
	if strings.TrimSpace(claims.InvoiceID) == "" ||
		strings.TrimSpace(claims.MerchantID) == "" ||
		strings.TrimSpace(claims.ShipmentID) == "" {
		return nil, ErrMalformedClaims
	}
	if !claims.PaymentStatus.IsValid() {
		return nil, ErrMalformedClaims
	}

	return claims, nil
}
