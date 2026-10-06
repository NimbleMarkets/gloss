package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPickNetworkFlags(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--listen", "192.168.1.42:0"}, {"--listen", "[::1]:8080"},
		{"--listen", "0.0.0.0:0", "--advertise-host", "laptop.example.ts.net"},
		{"--listen", "[::]:9090", "--advertise-host", "[fd7a:115c:a1e0::1]"},
	} {
		if _, _, err := parse(append([]string{"--pick-web"}, args...), io.Discard); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{
		{"--listen", ""}, {"--listen", "localhost:0"}, {"--listen", "en0:0"},
		{"--listen", ":8080"}, {"--listen", "127.0.0.1:65536"},
		{"--listen", "0.0.0.0:0"}, {"--listen", "[::]:0"},
		{"--advertise-host", ""}, {"--advertise-host", "https://laptop.ts.net"},
		{"--advertise-host", "laptop:80"}, {"--advertise-host", "laptop/path"},
		{"--advertise-host", "0.0.0.0"}, {"--advertise-host", "::"},
		{"--advertise-host", "user@laptop"}, {"--advertise-host", "laptop?x=1"},
		{"--advertise-host", "laptop#fragment"}, {"--advertise-host", "☕.example"},
		{"--advertise-host", "*.example"}, {"--advertise-host", "bad..example"},
		{"--advertise-host", "-bad.example"}, {"--advertise-host", "bad-.example"},
		{"--advertise-host", "127.1"}, {"--advertise-host", "2130706433"},
		{"--advertise-host", "laptop\n.example"},
		{"--listen", "224.0.0.1:0"}, {"--listen", "255.255.255.255:0"},
		{"--listen", "169.254.1.2:0"}, {"--listen", "[fe80::1%en0]:0"},
		{"--listen", "[::]:0", "--advertise-host", "192.168.1.42"},
		{"--listen", "0.0.0.0:0", "--advertise-host", "::1"},
		{"--listen", "192.168.1.42:0", "--advertise-host", "192.168.1.43"},
	} {
		if _, _, err := parse(append([]string{"--pick-web"}, args...), io.Discard); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, mode := range [][]string{nil, {"--serve"}, {"--pick"}, {"--resume", testToken}, {"--status", testToken}, {"--cancel", testToken}} {
		for _, flags := range [][]string{{"--listen", defaultPickListen}, {"--advertise-host", "localhost"}} {
			if _, _, err := parse(append(append([]string{}, mode...), flags...), io.Discard); err == nil {
				t.Errorf("accepted network flags outside web pick: %v %v", mode, flags)
			}
		}
	}
}

func TestPickNetworkAddressesAndHostPolicy(t *testing.T) {
	for _, tt := range []struct {
		listen, advertised, authority string
		allowed, denied               []string
	}{
		{defaultPickListen, "", "127.0.0.1:12345", []string{"127.0.0.1:12345", "LOCALHOST:12345"}, []string{"localhost:80", "127.0.0.2:12345"}},
		{"192.168.1.42:0", "Laptop.Example.TS.NET.", "laptop.example.ts.net:12345", []string{"192.168.1.42:12345", "LAPTOP.example.ts.net.:12345"}, []string{"laptop:12345", "localhost:12345"}},
		{"0.0.0.0:0", "laptop.example.ts.net", "laptop.example.ts.net:12345", []string{"laptop.example.ts.net:12345"}, []string{"0.0.0.0:12345", "127.0.0.1:12345", "192.168.1.42:12345"}},
		{"[::1]:0", "", "[::1]:12345", []string{"[0:0:0:0:0:0:0:1]:12345", "localhost:12345"}, []string{"127.0.0.1:12345", "[::1]:80"}},
		{"[::]:0", "fd7a:115c:a1e0::1", "[fd7a:115c:a1e0::1]:12345", []string{"[fd7a:115c:a1e0::1]:12345"}, []string{"[::]:12345", "[::1]:12345"}},
	} {
		t.Run(tt.listen+tt.advertised, func(t *testing.T) {
			n, err := parsePickNetwork(tt.listen, tt.advertised)
			if err != nil {
				t.Fatal(err)
			}
			a := n.access(12345)
			if a.authority != tt.authority {
				t.Fatalf("shared authority %q; want %q", a.authority, tt.authority)
			}
			for _, host := range tt.allowed {
				if !a.allows(host) {
					t.Errorf("refused configured host %q", host)
				}
			}
			for _, host := range append(tt.denied, "evil.example:12345", "user@"+tt.authority, tt.authority+"/", tt.authority+"?", "localhost:", "localhost:65536") {
				if a.allows(host) {
					t.Errorf("admitted foreign/malformed host %q", host)
				}
			}
		})
	}
	n, _ := parsePickNetwork(defaultPickListen, "localhost")
	if !n.access(80).allows("localhost") {
		t.Fatal("a browser may omit the default HTTP port")
	}
}

func TestPickOriginMustMatchRequestAuthority(t *testing.T) {
	for _, tt := range []struct {
		host, origin string
		want         bool
	}{
		{"laptop.ts.net:12345", "http://laptop.ts.net:12345", true},
		{"LAPTOP.ts.net:12345", "http://laptop.ts.net:12345", true},
		{"localhost:80", "http://localhost", true},
		{"[::1]:12345", "http://[0:0:0:0:0:0:0:1]:12345", true},
		{"laptop.ts.net:12345", "https://laptop.ts.net:12345", false},
		{"laptop.ts.net:12345", "http://laptop.ts.net:12346", false},
		{"laptop.ts.net:12345", "http://evil.example:12345", false},
		{"laptop.ts.net:12345", "http://laptop.ts.net:12345/", false},
		{"laptop.ts.net:12345", "http://laptop.ts.net:12345?", false},
		{"laptop.ts.net:12345", "http://user@laptop.ts.net:12345", false},
		{"laptop.ts.net:12345", "http://laptop.ts.net:12345#x", false},
		{"laptop.ts.net:12345", "null", false},
		{"laptop.ts.net:12345", "", false},
	} {
		r := httptest.NewRequest("POST", "http://"+tt.host+"/token/files", nil)
		r.Header.Set("Origin", tt.origin)
		if samePickOrigin(r) != tt.want {
			t.Errorf("host %q, origin %q; want %v", tt.host, tt.origin, tt.want)
		}
	}
	r := httptest.NewRequest("POST", "http://localhost:80/token/files", nil)
	if !samePickOrigin(r) {
		t.Fatal("token-bearing script clients may omit Origin")
	}
	r.Header.Add("Origin", "http://localhost")
	r.Header.Add("Origin", "http://evil.example")
	if samePickOrigin(r) {
		t.Fatal("accepted multiple origins")
	}
}

func TestWebPickAdvertisedDNSOverWildcard(t *testing.T) {
	s, err := serveWebPick(context.Background(), options{Listen: "0.0.0.0:0", AdvertiseHost: "laptop.example.ts.net"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); s.Discard() })
	u, _ := url.Parse(s.URL)
	if u.Hostname() != "laptop.example.ts.net" || u.Port() == "0" || u.Port() == "" {
		t.Fatalf("not a usable advertised URL: %s", s.URL)
	}
	// Dial the test's local socket while retaining the advertised Host and
	// Origin, exactly as a remote browser would. No live MagicDNS is required.
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", u.Port()))
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	for _, tt := range []struct {
		host, origin, path string
		want               int
	}{
		{u.Host, "http://" + u.Host, u.Path + "files", 200},
		{u.Host, "https://" + u.Host, u.Path + "files", 403},
		{"evil.example:" + u.Port(), "http://" + u.Host, u.Path + "files", 403},
		{"127.0.0.1:" + u.Port(), "", u.Path, 403},
		{u.Host, "http://" + u.Host, "/wrong-token/files", 404},
	} {
		r, _ := http.NewRequest("GET", "http://"+u.Host+tt.path, nil)
		r.Host = tt.host
		if tt.origin != "" {
			r.Header.Set("Origin", tt.origin)
		}
		r.Header.Set("X-Forwarded-Host", u.Host)
		r.Header.Set("X-Forwarded-Proto", "http")
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tt.want {
			t.Errorf("%+v: got %d", tt, response.StatusCode)
		}
	}
	// Real mutation through the advertised origin, preserving the token check.
	r, _ := http.NewRequest("POST", s.URL+"decline", nil)
	r.Header.Set("Origin", "http://"+u.Host)
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	s.webPick.mu.Lock()
	defer s.webPick.mu.Unlock()
	if response.StatusCode != 200 || s.webPick.state != statusDeclined {
		t.Fatal("same-origin mutation failed")
	}
}

func TestWebPickIPv6ListenerAndBusyPort(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	address := listener.Addr().String()
	s, err := serveWebPick(context.Background(), options{Listen: address})
	listener.Close()
	if err == nil {
		s.Close()
		t.Fatal("occupied port did not fail")
	}
	if !strings.Contains(err.Error(), "--listen") {
		t.Fatalf("bind failure lost context: %v", err)
	}
	s, err = serveWebPick(context.Background(), options{Listen: "[::1]:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if !strings.HasPrefix(s.URL, "http://[::1]:") {
		t.Fatalf("invalid IPv6 URL: %s", s.URL)
	}
	code, _ := get(t, s.URL, nil)
	if code != 200 {
		t.Fatalf("IPv6 page: %d", code)
	}
}
