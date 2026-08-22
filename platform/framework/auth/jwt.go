package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenClaims struct {
	TenantID      string        `json:"tid"`
	PrincipalKind PrincipalKind `json:"principal_kind"`
	ClientSurface string        `json:"surface"`
	PrincipalID   string        `json:"-"`
	LegacyUserID  string        `json:"uid,omitempty"`
	UsedLegacyID  bool          `json:"-"`
	SessionID     string        `json:"sid"`
	Realm         Realm         `json:"realm"`
	AuthVersion   uint64        `json:"auth_version"`
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

func (s *TokenService) Issue(tenantID, principalID, sessionID string, authVersion uint64) (string, time.Time, error) {
	return s.IssueFor(tenantID, PrincipalRef{Kind: PrincipalKindAccount, ID: principalID}, TenantConsoleSurface, TenantAudience, sessionID, authVersion)
}

func (s *TokenService) IssueFor(tenantID string, principal PrincipalRef, surface, audience, sessionID string, authVersion uint64) (string, time.Time, error) {
	if tenantID == "" || !principal.Valid() || surface == "" || audience == "" || sessionID == "" || authVersion == 0 {
		return "", time.Time{}, fmt.Errorf("tenant, principal, session and auth version are required")
	}
	now := time.Now().UTC()
	exp := now.Add(s.ttl)
	claims := TokenClaims{
		TenantID:      tenantID,
		PrincipalKind: principal.Kind,
		ClientSurface: surface,
		PrincipalID:   principal.ID,
		LegacyUserID:  principal.ID,
		SessionID:     sessionID,
		AuthVersion:   authVersion,
		Realm:         RealmTenant,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   principal.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Audience:  jwt.ClaimStrings{audience},
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
	return s.ParseFor(tokenString, TenantAudience)
}

func (s *TokenService) ParseFor(tokenString, audience string) (*TokenClaims, error) {
	if audience == "" {
		return nil, fmt.Errorf("token audience is required")
	}
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithAudience(audience))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if claims.Subject != "" && claims.LegacyUserID != "" && claims.Subject != claims.LegacyUserID {
		return nil, fmt.Errorf("token principal claims conflict")
	}
	claims.PrincipalID = claims.Subject
	if claims.PrincipalID == "" {
		claims.PrincipalID = claims.LegacyUserID
		claims.UsedLegacyID = claims.PrincipalID != ""
	}
	if claims.PrincipalKind == "" {
		claims.PrincipalKind = PrincipalKindAccount
	}
	if claims.ClientSurface == "" && audience == TenantAudience {
		claims.ClientSurface = TenantConsoleSurface
	}
	if claims.TenantID == "" || !(PrincipalRef{Kind: claims.PrincipalKind, ID: claims.PrincipalID}).Valid() || claims.ClientSurface == "" || claims.SessionID == "" || claims.Realm != RealmTenant || claims.AuthVersion == 0 {
		return nil, fmt.Errorf("token missing tenant, principal or session")
	}
	return claims, nil
}

// LooksLikeSessionToken classifies only the claim shape so middleware can keep
// non-session bearer tokens available to dedicated handlers. It never trusts
// or returns the unverified claim values.
func (s *TokenService) LooksLikeSessionToken(tokenString string) bool {
	claims := &TokenClaims{}
	_, _, err := new(jwt.Parser).ParseUnverified(tokenString, claims)
	if err != nil {
		return false
	}
	return claims.SessionID != "" || claims.AuthVersion != 0 || claims.PrincipalKind != "" || claims.ClientSurface != ""
}
