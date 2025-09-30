package utils

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWK represents a JSON Web Key
type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWKSValidator validates JWT tokens using JWKS
type JWKSValidator struct {
	jwksURL     string
	keys        map[string]*rsa.PublicKey
	mutex       sync.RWMutex
	lastRefresh time.Time
	refreshTTL  time.Duration
}

// NewJWKSValidator creates a new JWKS validator
func NewJWKSValidator(region, userPoolID string) *JWKSValidator {
	jwksURL := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s/.well-known/jwks.json", region, userPoolID)

	validator := &JWKSValidator{
		jwksURL:    jwksURL,
		keys:       make(map[string]*rsa.PublicKey),
		refreshTTL: 24 * time.Hour, // Refresh keys daily
	}

	// Load keys on initialization
	_ = validator.refreshKeys()

	return validator
}

// refreshKeys fetches and caches the public keys from JWKS endpoint
func (v *JWKSValidator) refreshKeys() error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	// Skip if recently refreshed
	if time.Since(v.lastRefresh) < v.refreshTTL {
		return nil
	}

	resp, err := http.Get(v.jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read JWKS response: %w", err)
	}

	var jwks JWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("failed to parse JWKS: %w", err)
	}

	// Convert JWKs to RSA public keys
	newKeys := make(map[string]*rsa.PublicKey)
	for _, jwk := range jwks.Keys {
		if jwk.Kty != "RSA" {
			continue
		}

		pubKey, err := v.jwkToRSAPublicKey(jwk)
		if err != nil {
			continue
		}

		newKeys[jwk.Kid] = pubKey
	}

	v.keys = newKeys
	v.lastRefresh = time.Now()

	return nil
}

// jwkToRSAPublicKey converts a JWK to an RSA public key
func (v *JWKSValidator) jwkToRSAPublicKey(jwk JWK) (*rsa.PublicKey, error) {
	// Decode N (modulus)
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode N: %w", err)
	}

	// Decode E (exponent)
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode E: %w", err)
	}

	// Convert bytes to big integers
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

// GetKey returns the public key for the given key ID
func (v *JWKSValidator) GetKey(kid string) (*rsa.PublicKey, error) {
	v.mutex.RLock()
	key, exists := v.keys[kid]
	v.mutex.RUnlock()

	if exists {
		return key, nil
	}

	// Key not found, try refreshing
	if err := v.refreshKeys(); err != nil {
		return nil, fmt.Errorf("failed to refresh keys: %w", err)
	}

	v.mutex.RLock()
	key, exists = v.keys[kid]
	v.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("key with kid %s not found", kid)
	}

	return key, nil
}

// ValidateToken validates a JWT token using JWKS
func (v *JWKSValidator) ValidateToken(tokenString string) (*jwt.Token, error) {
	// Parse token with custom key function
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Get key ID from token header
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("kid not found in token header")
		}

		// Get the public key
		return v.GetKey(kid)
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	return token, nil
}
