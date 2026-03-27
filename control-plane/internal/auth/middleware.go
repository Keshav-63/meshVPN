package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var (
	jwksCache     *ecdsa.PublicKey
	jwksCacheMux  sync.RWMutex
	jwksCacheTime time.Time
)

type UserRepository interface {
	GetByID(userID string) (domain.User, error)
	Upsert(user domain.User) error
}

type MiddlewareConfig struct {
	SupabaseURL     string
	SupabaseAnonKey string
	JWTSecret       string
	RequireAuth     bool
	UserRepo        UserRepository
}

type Claims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Provider string `json:"provider"`
	AppMeta  struct {
		Provider string `json:"provider"`
	} `json:"app_metadata"`
	UserMeta map[string]any `json:"user_metadata"`
	jwt.RegisteredClaims
}

// JWKSResponse represents the JWKS endpoint response
type JWKSResponse struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		X   string `json:"x"`
		Y   string `json:"y"`
		Crv string `json:"crv"`
	} `json:"keys"`
}

// fetchSupabasePublicKey fetches the ES256 public key from Supabase JWKS endpoint
func fetchSupabasePublicKey(supabaseURL, anonKey string) (*ecdsa.PublicKey, error) {
	// Check cache (valid for 1 hour)
	jwksCacheMux.RLock()
	if jwksCache != nil && time.Since(jwksCacheTime) < time.Hour {
		key := jwksCache
		jwksCacheMux.RUnlock()
		return key, nil
	}
	jwksCacheMux.RUnlock()

	// Fetch JWKS
	jwksURL := strings.TrimSuffix(supabaseURL, "/") + "/auth/v1/.well-known/jwks.json"

	// Create request with API key header
	req, err := http.NewRequest("GET", jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS request: %w", err)
	}

	// Add apikey header if provided
	if anonKey != "" {
		req.Header.Set("apikey", anonKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var jwks JWKSResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("no keys found in JWKS")
	}

	// Use the first ES256 key
	key := jwks.Keys[0]
	if key.Kty != "EC" || key.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported key type: %s %s", key.Kty, key.Crv)
	}

	// Decode x and y coordinates
	xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		return nil, fmt.Errorf("failed to decode x coordinate: %w", err)
	}

	yBytes, err := base64.RawURLEncoding.DecodeString(key.Y)
	if err != nil {
		return nil, fmt.Errorf("failed to decode y coordinate: %w", err)
	}

	// Construct ECDSA public key
	publicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}

	// Cache the key
	jwksCacheMux.Lock()
	jwksCache = publicKey
	jwksCacheTime = time.Now()
	jwksCacheMux.Unlock()

	logs.Infof("auth", "fetched and cached ES256 public key from Supabase JWKS")
	return publicKey, nil
}

// parseECDSAPublicKey parses a PEM-encoded ECDSA public key
func parseECDSAPublicKey(pemKey string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the public key")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an ECDSA public key")
	}

	return ecdsaPub, nil
}

// RequireSupabaseAuth validates Supabase JWT tokens for GitHub OAuth or email-based authentication
func RequireSupabaseAuth(cfg MiddlewareConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.RequireAuth {
			logs.Debugf("auth", "auth disabled, passing through path=%s", c.FullPath())
			c.Next()
			return
		}

		logs.Debugf("auth", "checking authorization header path=%s", c.FullPath())
		authz := strings.TrimSpace(c.GetHeader("Authorization"))
		if authz == "" {
			logs.Errorf("auth", "missing authorization header")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authz, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			logs.Errorf("auth", "invalid authorization header format")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		}

		if strings.TrimSpace(cfg.JWTSecret) == "" {
			logs.Errorf("auth", "SUPABASE_JWT_SECRET missing while REQUIRE_AUTH=true")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "auth misconfigured"})
			return
		}

		tokenText := strings.TrimSpace(parts[1])
		logs.Debugf("auth", "validating JWT token")
		claims := &Claims{}

		// Parse token to check the algorithm in header
		token, err := jwt.ParseWithClaims(tokenText, claims, func(t *jwt.Token) (any, error) {
			alg := t.Header["alg"]
			logs.Debugf("auth", "token algorithm: %v", alg)

			// Support both HMAC (HS256) and ECDSA (ES256) signing methods
			switch t.Method.(type) {
			case *jwt.SigningMethodHMAC:
				// HMAC signing (HS256) - Supabase legacy JWT secret
				// The secret is base64-encoded, decode it first
				secretBytes, decodeErr := base64.StdEncoding.DecodeString(cfg.JWTSecret)
				if decodeErr != nil {
					logs.Debugf("auth", "JWT secret is not base64, using as-is")
					// If not base64, use as raw bytes
					return []byte(cfg.JWTSecret), nil
				}
				logs.Debugf("auth", "using decoded JWT secret for HS256")
				return secretBytes, nil

			case *jwt.SigningMethodECDSA:
				// ECDSA signing (ES256) - fetch public key from Supabase JWKS
				// Only try JWKS if Supabase URL is configured
				if cfg.SupabaseURL != "" {
					logs.Infof("auth", "fetching ES256 public key from Supabase JWKS")
					return fetchSupabasePublicKey(cfg.SupabaseURL, cfg.SupabaseAnonKey)
				}
				// Fallback: try to parse as PEM-encoded public key
				return parseECDSAPublicKey(cfg.JWTSecret)

			default:
				return nil, fmt.Errorf("unexpected signing method: %v", alg)
			}
		})
		if err != nil || token == nil || !token.Valid {
			logs.Errorf("auth", "token validation failed: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		now := time.Now()
		if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(now) {
			logs.Errorf("auth", "token expired sub=%s", claims.Sub)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token expired"})
			return
		}

		provider := strings.ToLower(strings.TrimSpace(claims.Provider))
		if provider == "" {
			provider = strings.ToLower(strings.TrimSpace(claims.AppMeta.Provider))
		}

		// Allow GitHub OAuth and email-based authentication
		allowedProviders := map[string]bool{
			"github": true,
			"email":  true,
		}

		if provider == "" || !allowedProviders[provider] {
			logs.Errorf("auth", "unsupported provider sub=%s provider=%s", claims.Sub, provider)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "unsupported authentication provider"})
			return
		}

		logs.Infof("auth", "authenticated sub=%s email=%s provider=%s", claims.Sub, claims.Email, provider)
		c.Set("auth.sub", claims.Sub)
		c.Set("auth.email", claims.Email)
		c.Set("auth.provider", provider)

		// Load user from database or create if first login
		if cfg.UserRepo != nil {
			user, err := cfg.UserRepo.GetByID(claims.Sub)
			if err != nil {
				// User doesn't exist, create new user (first login)
				logs.Infof("auth", "creating new user sub=%s email=%s", claims.Sub, claims.Email)
				user = domain.User{
					UserID:       claims.Sub,
					Email:        claims.Email,
					Provider:     provider,
					IsSubscriber: false, // Default to free tier
				}
				if upsertErr := cfg.UserRepo.Upsert(user); upsertErr != nil {
					logs.Errorf("auth", "failed to create user: %v", upsertErr)
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize user"})
					return
				}
			}

			// Set user object and subscription status in context
			c.Set("auth.user", user)
			c.Set("auth.is_subscriber", user.IsSubscriber)
			logs.Debugf("auth", "loaded user sub=%s is_subscriber=%t", user.UserID, user.IsSubscriber)
		}

		c.Next()
	}
}
