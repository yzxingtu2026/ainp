package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/httptransport"
)

func TestHTTPTransportExchangeCarriesRequestAndAppliesCopiedCookies(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "https://app.example.com/api/auth/refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, exchange := httptransport.WithRequest(context.Background(), request)
	loaded, ok := httptransport.FromContext(ctx)
	if !ok || loaded != exchange || loaded.Request != request {
		t.Fatal("request exchange was not preserved in context")
	}

	cookie := &http.Cookie{Name: "refresh", Value: "secret", Path: "/api/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode}
	exchange.SetCookie(cookie)
	cookie.Value = "mutated"
	header := make(http.Header)
	exchange.Apply(header)
	got := header.Values("Set-Cookie")
	if len(got) != 1 || got[0] != "refresh=secret; Path=/api/; HttpOnly; Secure; SameSite=Lax" {
		t.Fatalf("Set-Cookie=%q", got)
	}
}
