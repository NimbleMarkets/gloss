package main

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const defaultPickListen = "127.0.0.1:0"

// pickNetwork separates the local socket from the host shared with the human.
// It resolves no names: a DNS name is an explicit advertised alias, never an
// instruction to bind a remote address or trust changing DNS during a request.
type pickNetwork struct {
	listen netip.AddrPort
	host   string
}

func parsePickNetwork(listen, advertised string) (pickNetwork, error) {
	addr, err := netip.ParseAddrPort(listen)
	if err != nil || addr.Addr().Zone() != "" {
		return pickNetwork{}, fmt.Errorf("--listen needs a literal IP and port, such as 192.168.1.42:0 or [::1]:0 (no interface names or IPv6 zones)")
	}
	addr = netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port())
	if addr.Addr().IsMulticast() || addr.Addr().IsLinkLocalUnicast() || addr.Addr() == netip.MustParseAddr("255.255.255.255") {
		return pickNetwork{}, fmt.Errorf("--listen needs a unicast or wildcard address; link-local addresses are not supported")
	}
	if advertised == "" {
		if addr.Addr().IsUnspecified() {
			return pickNetwork{}, fmt.Errorf("a wildcard --listen requires --advertise-host with the IP or DNS name the other device can reach")
		}
		advertised = addr.Addr().String()
	}
	host, err := pickHost(advertised)
	if err != nil {
		return pickNetwork{}, fmt.Errorf("--advertise-host: %w", err)
	}
	ip, ipErr := netip.ParseAddr(host)
	if addr.Addr().IsUnspecified() && (host == "localhost" || (ipErr == nil && ip.IsLoopback())) {
		return pickNetwork{}, fmt.Errorf("a wildcard --listen cannot advertise loopback; use a reachable LAN IP or DNS name, or bind loopback explicitly")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is4() != addr.Addr().Is4() || (!addr.Addr().IsUnspecified() && ip != addr.Addr()) {
			return pickNetwork{}, fmt.Errorf("--advertise-host IP must match --listen, or its address family for a wildcard listener")
		}
	}
	return pickNetwork{listen: addr, host: host}, nil
}

// pickHost accepts IP literals or ASCII DNS names (including MagicDNS names
// and punycode), but never URLs, ports, userinfo, wildcards, or scoped addresses.
func pickHost(host string) (string, error) {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		ip, err := netip.ParseAddr(host[1 : len(host)-1])
		if err != nil || !ip.Is6() {
			return "", fmt.Errorf("brackets are only for IPv6 addresses")
		}
		host = ip.String()
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip == netip.MustParseAddr("255.255.255.255") {
			return "", fmt.Errorf("use a reachable unicast IP, without an IPv6 zone")
		}
		return ip.String(), nil
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || len(host) > 253 {
		return "", fmt.Errorf("use an IP or ASCII DNS name, without a scheme, port, or path")
	}
	allNumeric := true
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid DNS name")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", fmt.Errorf("use an IP or ASCII DNS name, without a scheme, port, or path")
			}
			allNumeric = allNumeric && c >= '0' && c <= '9'
		}
	}
	if allNumeric {
		return "", fmt.Errorf("use a dotted-decimal IP address or a DNS name")
	}
	return host, nil
}

// The access policy is fixed before serving. A wildcard listener does not
// admit arbitrary Host headers, and forwarded headers cannot broaden it.
type pickAccess struct {
	authority string
	allowed   map[string]bool
}

func (n pickNetwork) access(port int) pickAccess {
	portText := strconv.Itoa(port)
	a := pickAccess{authority: net.JoinHostPort(n.host, portText), allowed: make(map[string]bool)}
	a.allowed[a.authority] = true
	if !n.listen.Addr().IsUnspecified() {
		a.allowed[net.JoinHostPort(n.listen.Addr().String(), portText)] = true
		if n.listen.Addr().IsLoopback() {
			a.allowed[net.JoinHostPort("localhost", portText)] = true
		}
	}
	return a
}

func (a pickAccess) allows(authority string) bool {
	key, err := pickAuthority(authority)
	return err == nil && a.allowed[key]
}

// Browsers may omit :80, case-fold DNS, or compress IPv6. Compare canonical
// authorities rather than widening the policy to every port on a hostname.
func pickAuthority(authority string) (string, error) {
	u, err := url.Parse("http://" + authority)
	if err != nil || u.Host != authority || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("invalid HTTP authority")
	}
	host, err := pickHost(u.Hostname())
	if err != nil {
		return "", err
	}
	port := u.Port()
	if port == "" {
		if strings.HasSuffix(authority, ":") {
			return "", fmt.Errorf("empty port")
		}
		port = "80"
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return "", fmt.Errorf("invalid port")
	}
	return net.JoinHostPort(host, strconv.Itoa(int(n))), nil
}

func samePickOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true // Local/script clients still need the session token.
	}
	if len(origins) != 1 {
		return false
	}
	u, err := url.Parse(origins[0])
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	origin, err := pickAuthority(u.Host)
	if err != nil {
		return false
	}
	host, err := pickAuthority(r.Host)
	return err == nil && origin == host
}
