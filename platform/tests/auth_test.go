package tests

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

func TestTokenServiceIssuesTenantRealmAndAudience(t *testing.T) {
	svc, err := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.Issue("tenant-1", "user-1", "session-1", 1)
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
	if claims.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %s", claims.SessionID)
	}
	if claims.PrincipalID != "user-1" || claims.Subject != "user-1" || claims.LegacyUserID != "user-1" {
		t.Fatalf("unexpected principal claims: %#v", claims)
	}
}

func TestTokenServiceClassifiesSessionAndAttachmentTokens(t *testing.T) {
	const (
		secret = "shared-test-secret"
		issuer = "ainp-test"
	)
	sessions, err := auth.NewTokenService(secret, issuer, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sessionToken, _, err := sessions.Issue("tenant-1", "user-1", "session-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !sessions.LooksLikeSessionToken(sessionToken) {
		t.Fatal("valid session token was not classified as a session token")
	}
	if !sessions.LooksLikeSessionToken(sessionToken + "x") {
		t.Fatal("session token with a tampered signature must remain classified as a session token")
	}

	transfers, err := attachment.NewTransferTokenService(secret, issuer)
	if err != nil {
		t.Fatal(err)
	}
	transferToken, _, _, err := transfers.Issue(attachment.IssueTransferInput{
		TenantID:       "tenant-1",
		PrincipalID:    "user-1",
		PrincipalKind:  string(auth.PrincipalKindAccount),
		ClientSurface:  auth.TenantConsoleSurface,
		SourceAudience: auth.TenantAudience,
		AttachmentID:   "attachment-1",
		Op:             attachment.OpUpload,
		Size:           12,
		TTL:            time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sessions.LooksLikeSessionToken(transferToken) {
		t.Fatal("attachment transfer token was classified as a session token")
	}
}

func TestTokenServiceRejectsLegacyTokenWithoutAudience(t *testing.T) {
	svc, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	claims := auth.TokenClaims{TenantID: "tenant-1", LegacyUserID: "user-1", SessionID: "session-1", Realm: auth.RealmTenant, AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{
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
	claims := auth.TokenClaims{TenantID: "tenant-1", LegacyUserID: "user-1", SessionID: "session-1", Realm: auth.RealmTenant, RegisteredClaims: jwt.RegisteredClaims{
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

func TestTokenServiceRejectsTokenWithoutSession(t *testing.T) {
	svc, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	claims := auth.TokenClaims{TenantID: "tenant-1", LegacyUserID: "user-1", Realm: auth.RealmTenant, AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "ainp-test", Audience: jwt.ClaimStrings{auth.TenantAudience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if _, err := svc.Parse(token); err == nil {
		t.Fatal("expected token without session_id to be rejected")
	}
}

func TestTokenServiceRejectsConflictingSubjectAndLegacyUID(t *testing.T) {
	svc, _ := auth.NewTokenService("test-secret", "ainp-test", time.Hour)
	claims := auth.TokenClaims{TenantID: "tenant-1", LegacyUserID: "legacy", SessionID: "session-1", Realm: auth.RealmTenant, AuthVersion: 1, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "ainp-test", Subject: "canonical", Audience: jwt.ClaimStrings{auth.TenantAudience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	if _, err := svc.Parse(token); err == nil {
		t.Fatal("expected conflicting sub and uid to be rejected")
	}
}
