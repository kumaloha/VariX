package httputil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"
)

// NewPublicHTTPClient returns an HTTP client for untrusted article/feed URLs.
// It rejects localhost, private/link-local addresses, metadata services, and
// non-HTTP schemes before each request and redirect.
func NewPublicHTTPClient(timeout time.Duration, jar http.CookieJar) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = publicDialer().DialContext
	client := &http.Client{
		Timeout:       timeout,
		Transport:     publicRoundTripper{base: transport},
		CheckRedirect: publicRedirectPolicy,
	}
	if jar != nil {
		client.Jar = jar
	}
	return client
}

func ValidatePublicURL(ctx context.Context, raw string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	return validatePublicRequest(req)
}

type publicRoundTripper struct {
	base http.RoundTripper
}

func (t publicRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validatePublicRequest(req); err != nil {
		return nil, err
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func publicRedirectPolicy(req *http.Request, _ []*http.Request) error {
	return validatePublicRequest(req)
}

func validatePublicRequest(req *http.Request) error {
	if req == nil || req.URL == nil {
		return fmt.Errorf("blocked outbound request: missing URL")
	}
	scheme := strings.ToLower(req.URL.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("blocked outbound request: unsupported scheme %q", req.URL.Scheme)
	}
	host := strings.TrimSpace(req.URL.Hostname())
	if host == "" {
		return fmt.Errorf("blocked outbound request: missing host")
	}
	if isLocalhostName(host) {
		return fmt.Errorf("blocked outbound request to localhost host %q", host)
	}
	if addr, ok, err := parseHostAddr(host); err != nil {
		return err
	} else if ok {
		return validatePublicAddr(addr, host)
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	resolver := net.DefaultResolver
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("blocked outbound request: resolve %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("blocked outbound request: resolve %q returned no addresses", host)
	}
	for _, addr := range addrs {
		if err := validatePublicAddr(addr, host); err != nil {
			return err
		}
	}
	return nil
}

func publicDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			addr, ok, err := parseHostAddr(host)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			return validatePublicAddr(addr, host)
		},
	}
}

func parseHostAddr(host string) (netip.Addr, bool, error) {
	host = strings.Trim(host, "[]")
	if zone := strings.IndexByte(host, '%'); zone >= 0 {
		host = host[:zone]
	}
	addr, err := netip.ParseAddr(host)
	if err == nil {
		return addr.Unmap(), true, nil
	}
	if strings.Contains(host, ":") {
		return netip.Addr{}, false, fmt.Errorf("blocked outbound request: invalid IP host %q", host)
	}
	return netip.Addr{}, false, nil
}

func isLocalhostName(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

func validatePublicAddr(addr netip.Addr, label string) error {
	addr = addr.Unmap()
	if addr == (netip.Addr{}) ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return fmt.Errorf("blocked outbound request to private or local address %s for host %q", addr, label)
	}
	if addr == netip.MustParseAddr("100.100.100.200") ||
		addr == netip.MustParseAddr("169.254.169.254") {
		return fmt.Errorf("blocked outbound request to metadata address %s for host %q", addr, label)
	}
	return nil
}
