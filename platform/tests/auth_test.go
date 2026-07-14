package tests

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

func TestTokenServiceIssuesTenantRealmAndAudience(t *testing.T) {
	svc, err := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.Issue("tenant-1", "user-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Realm != auth.RealmTenant || len(claims.Audience) != 1 || claims.Audience[0] != auth.TenantAudience {
		t.Fatalf("unexpected realm/audience: %q %#v", claims.Realm, claims.Audience)
	}
	if claims.AuthVersion != 1 {
		t.Fatalf("unexpected auth version: %d", claims.AuthVersion)
	}
}

func TestTokenServiceRejectsLegacyTokenWithoutAudience(t *testing.T) {
	svc, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	claims := auth.TokenClaims{TenantID: "tenant-1", UserID: "user-1", Realm: auth.RealmTenant, AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "ainp-test", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Parse(token); err == nil {
		t.Fatal("expected token without tenant audience to be rejected")
	}
}

func TestTokenServiceRejectsTokenWithoutAuthVersion(t *testing.T) {
	svc, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	claims := auth.TokenClaims{TenantID: "tenant-1", UserID: "user-1", Realm: auth.RealmTenant, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "ainp-test", Audience: jwt.ClaimStrings{auth.TenantAudience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Parse(token); err == nil {
		t.Fatal("expected token without auth_version to be rejected")
	}
}
