package middleware

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const userIDKey = "user_id"

func Auth(database *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		sum := sha256.Sum256([]byte(token))
		hash := hex.EncodeToString(sum[:])

		var userID string
		err := database.QueryRowContext(c.Request.Context(), `
			SELECT user_id
			FROM api_tokens
			WHERE token_hash = $1 AND (expires_at IS NULL OR expires_at > $2)
		`, hash, time.Now().UTC()).Scan(&userID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
			return
		}
		c.Set(userIDKey, userID)
		c.Next()
	}
}

func UserID(c *gin.Context) string {
	userID, _ := c.Get(userIDKey)
	if value, ok := userID.(string); ok {
		return value
	}
	return ""
}
