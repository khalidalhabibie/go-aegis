package webhooks

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type TargetPolicy struct {
	AllowedHosts        []string
	AllowPrivateTargets bool
}

type lookupIPAddrsFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

func validateDispatchTarget(ctx context.Context, targetURL *url.URL, policy TargetPolicy, lookupIPAddrs lookupIPAddrsFunc) error {
	hostname := normalizeHostname(targetURL.Hostname())
	if hostname == "" {
		return fmt.Errorf("target URL must include a valid hostname")
	}

	if len(policy.AllowedHosts) > 0 && !isAllowedTargetHost(hostname, policy.AllowedHosts) {
		return fmt.Errorf("target host %q is not in the allowed callback host list", hostname)
	}

	if policy.AllowPrivateTargets {
		return nil
	}

	if isPrivateHostnameLabel(hostname) {
		return fmt.Errorf("target host %q points to localhost or a private network label", hostname)
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		if isPrivateOrLocalIP(ip) {
			return fmt.Errorf("target host %q resolves to a private or local IP", hostname)
		}

		return nil
	}

	if lookupIPAddrs == nil {
		lookupIPAddrs = net.DefaultResolver.LookupIPAddr
	}

	ipAddrs, err := lookupIPAddrs(ctx, hostname)
	if err != nil {
		return fmt.Errorf("resolve target host %q: %w", hostname, err)
	}

	if len(ipAddrs) == 0 {
		return fmt.Errorf("resolve target host %q: no IP addresses returned", hostname)
	}

	for _, ipAddr := range ipAddrs {
		if isPrivateOrLocalIP(ipAddr.IP) {
			return fmt.Errorf("target host %q resolved to disallowed IP %s", hostname, ipAddr.IP.String())
		}
	}

	return nil
}

func isAllowedTargetHost(hostname string, allowedHosts []string) bool {
	normalizedHost := normalizeHostname(hostname)
	for _, candidate := range allowedHosts {
		allowed := normalizeHostname(candidate)
		if allowed == "" {
			continue
		}

		if normalizedHost == allowed || strings.HasSuffix(normalizedHost, "."+allowed) {
			return true
		}
	}

	return false
}

func isPrivateHostnameLabel(hostname string) bool {
	switch {
	case hostname == "localhost":
		return true
	case strings.HasSuffix(hostname, ".localhost"),
		strings.HasSuffix(hostname, ".local"),
		strings.HasSuffix(hostname, ".internal"):
		return true
	default:
		return false
	}
}

func isPrivateOrLocalIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

func normalizeHostname(hostname string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hostname)), ".")
}
