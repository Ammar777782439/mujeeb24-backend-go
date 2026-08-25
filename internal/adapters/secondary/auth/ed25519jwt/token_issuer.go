package ed25519jwt

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	PrivateKeyBase64 string
	PublicKeyBase64  string
	Issuer           string
	AccessTTL        time.Duration
}

type Issuer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	issuer     string
	accessTTL  time.Duration
}

func New(config Config) (*Issuer, error) {
	privateRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(config.PrivateKeyBase64))
	if err != nil {
		return nil, errors.New("JWT private key must be base64")
	}
	publicRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(config.PublicKeyBase64))
	if err != nil {
		return nil, errors.New("JWT public key must be base64")
	}
	if len(privateRaw) != ed25519.PrivateKeySize || len(publicRaw) != ed25519.PublicKeySize {
		return nil, errors.New("JWT Ed25519 key lengths are invalid")
	}
	if config.Issuer == "" {
		config.Issuer = "mujeeb24"
	}
	if config.AccessTTL <= 0 {
		return nil, errors.New("JWT access TTL must be positive")
	}
	return &Issuer{privateKey: ed25519.PrivateKey(privateRaw), publicKey: ed25519.PublicKey(publicRaw), issuer: config.Issuer, accessTTL: config.AccessTTL}, nil
}

func (i *Issuer) IssueAccessToken(_ context.Context, principalID commands.PrincipalID, now time.Time) (string, time.Time, error) {
	if i == nil || principalID == "" {
		return "", time.Time{}, errors.New("JWT issuer or principal is missing")
	}
	expiresAt := now.Add(i.accessTTL)
	claims := jwt.RegisteredClaims{Issuer: i.issuer, Subject: string(principalID), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiresAt)}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := token.SignedString(i.privateKey)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (i *Issuer) VerifyAccessToken(_ context.Context, raw string) (commands.PrincipalID, error) {
	if i == nil {
		return "", errors.New("JWT verifier is missing")
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("unexpected JWT signing method")
		}
		return i.publicKey, nil
	}, jwt.WithIssuer(i.issuer), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || strings.TrimSpace(claims.Subject) == "" {
		return "", errors.New("invalid access token")
	}
	return commands.PrincipalID(claims.Subject), nil
}
