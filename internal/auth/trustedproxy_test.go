package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyClientIP(t *testing.T) {
	prefixes, err := parseTrustedProxyPrefixes("10.20.0.2/32,2001:db8:1::2/128")
	if err != nil {
		t.Fatal(err)
	}
	rl := &RateLimiter{trustedProxies: prefixes}
	for _, tc := range []struct{ peer, xff, want string }{
		{"198.51.100.8:443", "1.2.3.4", "198.51.100.8"},
		{"10.20.0.2:443", "198.51.100.8", "198.51.100.8"},
		{"10.20.0.2:443", "1.2.3.4, 198.51.100.8", "198.51.100.8"},
		{"10.20.0.2:443", "198.51.100.8, 2001:db8:1::2", "198.51.100.8"},
		{"[::ffff:10.20.0.2]:443", "::ffff:198.51.100.8", "198.51.100.8"},
		{"[2001:db8:1::2]:443", "2001:db8:2::8", "2001:db8:2::8"},
		{"10.20.0.2:443", "198.51.100.8, garbage", "10.20.0.2"},
		{"10.20.0.2:443", "10.20.0.2", "10.20.0.2"},
		{"10.20.0.2:443", "", "10.20.0.2"},
	} {
		r := httptest.NewRequest("GET", "/v1/geocode", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.xff)
		if got := rl.requestIP(r); got != tc.want {
			t.Errorf("peer=%s xff=%s got=%s want=%s", tc.peer, tc.xff, got, tc.want)
		}
	}
	for _, raw := range []string{"garbage", "10.20.0.2/32,", "0.0.0.0/0", "::/0"} {
		if _, err := parseTrustedProxyPrefixes(raw); err == nil {
			t.Errorf("accepted unsafe config %q", raw)
		}
	}
}

func TestTrustedProxyRateLimitSeparatesClients(t *testing.T) {
	t.Setenv("B2D_TRUSTED_PROXY_CIDRS", "10.20.0.2/32")
	rl := NewRateLimiter()
	h := rl.IPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	request := func(client string) int {
		r := httptest.NewRequest("GET", "/v1/geocode", nil)
		r.RemoteAddr = "10.20.0.2:1234"
		r.Header.Set("X-Forwarded-For", client)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	limited := false
	for i := 0; i < 100; i++ {
		if request("198.51.100.1") == 429 {
			limited = true
			break
		}
	}
	if !limited || request("198.51.100.2") != 204 {
		t.Fatal("clients behind trusted proxy share bucket")
	}
}

func TestKeyLimiterStillAppliesAcrossDifferentIPs(t *testing.T) {
	rl := NewRateLimiter()
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	request := func(id int64, peer string) int {
		r := httptest.NewRequest("GET", "/v1/geocode", nil)
		r.RemoteAddr = peer
		ctx := context.WithValue(r.Context(), ctxKeyAPIKeyID, id)
		ctx = context.WithValue(ctx, ctxKeyRateRPS, 1)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r.WithContext(ctx))
		return w.Code
	}
	for i, tc := range []struct {
		id   int64
		peer string
		want int
	}{
		{1, "198.51.100.1:1234", 204},
		{1, "198.51.100.2:1234", 204},
		{1, "198.51.100.3:1234", 429},
		{2, "198.51.100.3:1234", 204},
	} {
		if got := request(tc.id, tc.peer); got != tc.want {
			t.Fatalf("request %d: key=%d peer=%s got=%d want=%d", i+1, tc.id, tc.peer, got, tc.want)
		}
	}
}
