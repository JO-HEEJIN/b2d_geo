package auth

import (
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

func trustedProxyPrefixesFromEnv() []netip.Prefix {
	prefixes, err := parseTrustedProxyPrefixes(os.Getenv("B2D_TRUSTED_PROXY_CIDRS"))
	if err != nil {
		// Fail closed: a typo must not broaden the set of trusted senders.
		log.Print("rate limit: invalid B2D_TRUSTED_PROXY_CIDRS; forwarding headers ignored")
		return nil
	}
	return prefixes
}

func parseTrustedProxyPrefixes(raw string) ([]netip.Prefix, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out []netip.Prefix
	for _, value := range strings.Split(raw, ",") {
		p, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil || p.Bits() == 0 {
			return nil, fmt.Errorf("trusted proxies must be explicit non-universal CIDRs")
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

func (rl *RateLimiter) trustedProxy(ip netip.Addr) bool {
	for _, prefix := range rl.trustedProxies {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func (rl *RateLimiter) requestIP(r *http.Request) string {
	peerText := clientIP(r)
	peer, err := netip.ParseAddr(peerText)
	if err != nil {
		return peerText
	}
	peer = peer.Unmap()
	if !rl.trustedProxy(peer) {
		return peer.String()
	}
	xff := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if len(xff) == 0 || len(xff) > 4096 {
		return peer.String()
	}
	hops := strings.Split(xff, ",")
	if len(hops) > 32 {
		return peer.String()
	}
	// Walk backwards from the actual TCP peer. Stop at the first untrusted
	// hop; values further left may have been supplied by that client.
	for i := len(hops) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return peer.String()
		}
		ip = ip.Unmap()
		if !rl.trustedProxy(ip) {
			return ip.String()
		}
	}
	return peer.String() // No independently identified client: aggregate safely.
}
