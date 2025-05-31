// internal/auth/auth.go

package auth

import (
	"errors"
	"time"

	"github.com/dgrijalva/jwt-go"
)

// JWTSecret holds the signing key (set by config.Init).
var JWTSecret []byte

// Init reads the JWT secret from environment and caches it.
func Init(secret string) {
	JWTSecret = []byte(secret)
}

// Claims defines the JWT payload for our admin users.
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.StandardClaims
}

// GenerateJWT creates a signed token valid for the given duration (e.g. 24h).
func GenerateJWT(userID, username, role string) (string, error) {
	ttl := 24 * time.Hour
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		StandardClaims: jwt.StandardClaims{
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(ttl).Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JWTSecret)
}

// ParseAndVerify validates the token string and returns its claims.
func ParseAndVerify(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		// ensure HS256
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return JWTSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
