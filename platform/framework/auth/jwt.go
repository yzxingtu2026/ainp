package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenClaims struct {
	TenantID    string   `json:"tid"`
	UserID      string   `json:"uid"`
	Realm       Realm    `json:"realm"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"perms,omitempty"`
	jwt.RegisteredClaims
}

const TenantAudience = "ainp:tenant-console"

type TokenService struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

func NewTokenService(secret, issuer string, ttl time.Duration) (*TokenService, error) {
	if secret == "" {
		return nil, fmt.Errorf("jwt secret is required")
	}
	if issuer == "" {
		issuer = "ainp"
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &TokenService{
		secret: []byte(secret),
		issuer: issuer,
		ttl:    ttl,
	}, nil
}

func (s *TokenService) TTL() time.Duration { return s.ttl }

func (s *TokenService) Issue(tenantID, userID string, roles, permissions []string) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(s.ttl)
	claims := TokenClaims{
		TenantID:    tenantID,
		UserID:      userID,
		Roles:       roles,
		Permissions: permissions,
		Realm:       RealmTenant,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Audience:  jwt.ClaimStrings{TenantAudience},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

func (s *TokenService) Parse(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithAudience(TenantAudience))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if claims.TenantID == "" || claims.UserID == "" || claims.Realm != RealmTenant {
		return nil, fmt.Errorf("token missing tenant or user")
	}
	return claims, nil
}
