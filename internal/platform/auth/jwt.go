package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"trackr/internal/platform/config"
)

type Claims struct {
	UserID         string   `json:"uid"`
	OrganizationID string   `json:"oid"`
	Role           string   `json:"role"`
	Email          string   `json:"email"`
	Scopes         []string `json:"scp"`
	jwt.RegisteredClaims
}

type TokenService struct {
	config config.JWTConfig
}

func NewTokenService(cfg config.JWTConfig) *TokenService {
	return &TokenService{config: cfg}
}

func (s *TokenService) GenerateAccessToken(userID, orgID, role, email string) (string, error) {
	claims := Claims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.config.AccessTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "trackr",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.Secret))
}

func (s *TokenService) GenerateRefreshToken(userID string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.config.RefreshTokenTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "trackr",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.Secret))
}

func (s *TokenService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.config.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
