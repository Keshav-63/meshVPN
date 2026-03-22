package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type UserRepository interface {
	GetByID(userID string) (domain.User, error)
	Upsert(user domain.User) error
}

type MiddlewareConfig struct {
	JWTSecret   string
	RequireAuth bool
	UserRepo    UserRepository
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

func RequireSupabaseGitHub(cfg MiddlewareConfig) gin.HandlerFunc {
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
		token, err := jwt.ParseWithClaims(tokenText, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(cfg.JWTSecret), nil
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
		if provider != "github" {
			logs.Errorf("auth", "non-github provider rejected sub=%s provider=%s", claims.Sub, provider)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "only GitHub login is allowed"})
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
