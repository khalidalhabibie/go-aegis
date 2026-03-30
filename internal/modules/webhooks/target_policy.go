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
	_, _, _, err := resolveDispatchTarget(ctx, targetURL, policy, lookupIPAddrs)
	return err
}

func resolveDispatchTarget(ctx context.Context, targetURL *url.URL, policy TargetPolicy, lookupIPAddrs lookupIPAddrsFunc) (string, string, []net.IP, error) {
	hostname := normalizeHostname(targetURL.Hostname())
	if hostname == "" {
		return "", "", nil, fmt.Errorf("target URL must include a valid hostname")
	}

	if len(policy.AllowedHosts) > 0 && !isAllowedTargetHost(hostname, policy.AllowedHosts) {
		return "", "", nil, fmt.Errorf("target host %q is not in the allowed callback host list", hostname)
	}

	port, err := resolveTargetPort(targetURL)
	if err != nil {
		return "", "", nil, err
	}

	if policy.AllowPrivateTargets {
		if ip := net.ParseIP(hostname); ip != nil {
			return hostname, port, []net.IP{ip}, nil
		}

		ipAddrs, err := resolveTargetIPAddrs(ctx, hostname, lookupIPAddrs)
		if err != nil {
			return "", "", nil, err
		}

		return hostname, port, extractIPs(ipAddrs), nil
	}

	if isPrivateHostnameLabel(hostname) {
		return "", "", nil, fmt.Errorf("target host %q points to localhost or a private network label", hostname)
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		if isPrivateOrLocalIP(ip) {
			return "", "", nil, fmt.Errorf("target host %q resolves to a private or local IP", hostname)
		}

		return hostname, port, []net.IP{ip}, nil
	}

	ipAddrs, err := resolveTargetIPAddrs(ctx, hostname, lookupIPAddrs)
	if err != nil {
		return "", "", nil, err
	}

	for _, ipAddr := range ipAddrs {
		if isPrivateOrLocalIP(ipAddr.IP) {
			return "", "", nil, fmt.Errorf("target host %q resolved to disallowed IP %s", hostname, ipAddr.IP.String())
		}
	}

	return hostname, port, extractIPs(ipAddrs), nil
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

func resolveTargetPort(targetURL *url.URL) (string, error) {
	if port := targetURL.Port(); port != "" {
		return port, nil
	}

	switch strings.ToLower(targetURL.Scheme) {
	case "http":
		return "80", nil
	case "https":
		return "443", nil
	default:
		return "", fmt.Errorf("target URL scheme %q is not supported", targetURL.Scheme)
	}
}

func resolveTargetIPAddrs(ctx context.Context, hostname string, lookupIPAddrs lookupIPAddrsFunc) ([]net.IPAddr, error) {
	if lookupIPAddrs == nil {
		lookupIPAddrs = net.DefaultResolver.LookupIPAddr
	}

	ipAddrs, err := lookupIPAddrs(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("resolve target host %q: %w", hostname, err)
	}

	if len(ipAddrs) == 0 {
		return nil, fmt.Errorf("resolve target host %q: no IP addresses returned", hostname)
	}

	return ipAddrs, nil
}

func extractIPs(ipAddrs []net.IPAddr) []net.IP {
	ips := make([]net.IP, 0, len(ipAddrs))
	for _, ipAddr := range ipAddrs {
		ips = append(ips, ipAddr.IP)
	}

	return ips
}
