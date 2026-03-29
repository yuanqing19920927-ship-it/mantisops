package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"mantisops/server/internal/store"
)

// --- Token Version Cache ---

type TokenVersionCache struct {
	mu    sync.RWMutex
	cache map[int64]int64
	us    *store.UserStore
}

func NewTokenVersionCache(us *store.UserStore) *TokenVersionCache {
	return &TokenVersionCache{cache: make(map[int64]int64), us: us}
}

func (c *TokenVersionCache) Get(userID int64) (int64, error) {
	c.mu.RLock()
	if v, ok := c.cache[userID]; ok {
		c.mu.RUnlock()
		return v, nil
	}
	c.mu.RUnlock()

	v, err := c.us.GetTokenVersion(userID)
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	c.cache[userID] = v
	c.mu.Unlock()
	return v, nil
}

func (c *TokenVersionCache) Invalidate(userID int64) {
	c.mu.Lock()
	delete(c.cache, userID)
	c.mu.Unlock()
}

// --- Auth Handler ---

type AuthHandler struct {
	userStore *store.UserStore
	jwtSecret []byte
	tvCache   *TokenVersionCache
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type jwtPayload struct {
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
	Role          string `json:"role"`
	TokenVersion  int64  `json:"token_version"`
	MustChangePwd bool   `json:"must_change_pwd"`
	Exp           int64  `json:"exp"`
}

func NewAuthHandler(userStore *store.UserStore, secret string, tvCache *TokenVersionCache) *AuthHandler {
	return &AuthHandler{userStore: userStore, jwtSecret: []byte(secret), tvCache: tvCache}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	c.Set("audit_username", req.Username)

	// Constant-time: always run bcrypt even if user doesn't exist (prevents timing enumeration)
	user, err := h.userStore.GetByUsername(req.Username)
	if err != nil {
		// User not found — compare against a dummy hash to keep constant timing
		bcrypt.CompareHashAndPassword([]byte("$2a$10$000000000000000000000uDummyHashForTimingConsistencyOnly."), []byte(req.Password))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if !user.Enabled {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := h.generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":           token,
		"username":        user.Username,
		"role":            user.Role,
		"display_name":    user.DisplayName,
		"must_change_pwd": user.MustChangePwd,
	})
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, _ := c.Get("user_id")
	user, err := h.userStore.GetByID(userID.(int64))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id":         user.ID,
		"username":        user.Username,
		"role":            user.Role,
		"display_name":    user.DisplayName,
		"must_change_pwd": user.MustChangePwd,
	})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old_password and new_password required"})
		return
	}

	userID := c.GetInt64("user_id")
	user, err := h.userStore.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "old password incorrect"})
		return
	}

	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 6 characters"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash failed"})
		return
	}

	if err := h.userStore.UpdatePassword(userID, string(hash)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}
	h.userStore.IncrementTokenVersion(userID)
	h.tvCache.Invalidate(userID)

	// Re-fetch to get updated fields
	user, err = h.userStore.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh user"})
		return
	}
	token, err := h.generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":           token,
		"username":        user.Username,
		"role":            user.Role,
		"display_name":    user.DisplayName,
		"must_change_pwd": user.MustChangePwd,
	})
}

func (h *AuthHandler) JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		payload, err := h.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Check token version
		currentVersion, err := h.tvCache.Get(payload.UserID)
		if err != nil || payload.TokenVersion < currentVersion {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
			c.Abort()
			return
		}

		// Must change password interception
		if payload.MustChangePwd {
			path := c.Request.URL.Path
			if path != "/api/v1/auth/me" && path != "/api/v1/auth/password" {
				c.JSON(http.StatusForbidden, gin.H{"error": "must_change_password"})
				c.Abort()
				return
			}
		}

		c.Set("username", payload.Username)
		c.Set("user_id", payload.UserID)
		c.Set("role", payload.Role)
		c.Next()
	}
}

// --- JWT generation/validation (same HS256 algorithm, extended payload) ---

func (h *AuthHandler) generateToken(user *store.User) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := jwtPayload{
		UserID:        user.ID,
		Username:      user.Username,
		Role:          user.Role,
		TokenVersion:  user.TokenVersion,
		MustChangePwd: user.MustChangePwd,
		Exp:           time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	payloadEnc := base64.RawURLEncoding.EncodeToString(payloadBytes)
	sigInput := header + "." + payloadEnc
	mac := hmac.New(sha256.New, h.jwtSecret)
	mac.Write([]byte(sigInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return sigInput + "." + sig, nil
}

// ValidateToken validates a JWT token and returns the payload (exported for WS auth)
func (h *AuthHandler) ValidateToken(token string) (*jwtPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}
	sigInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, h.jwtSecret)
	mac.Write([]byte(sigInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var payload jwtPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, err
	}
	if time.Now().Unix() > payload.Exp {
		return nil, fmt.Errorf("token expired")
	}
	return &payload, nil
}

// HashPassword generates a bcrypt hash (exported for use by migration and user_handler).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
