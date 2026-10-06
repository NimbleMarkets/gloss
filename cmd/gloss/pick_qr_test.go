package main

import (
	"bytes"
	"testing"
)

func TestPickQRDoesNotAdvertiseLoopbackAsPhoneHandoff(t *testing.T) {
	for _, tt := range []struct {
		listen, host string
		want         bool
	}{
		{defaultPickListen, "", false},
		{defaultPickListen, "laptop.ts.net", false},
		{"[::1]:0", "", false},
		{"0.0.0.0:0", "127.0.0.1", false},
		{"0.0.0.0:0", "localhost", false},
		{"192.168.9.216:0", "", true},
		{"0.0.0.0:0", "laptop.ts.net", true},
	} {
		opts := options{Listen: tt.listen, AdvertiseHost: tt.host}
		if networkPick(opts) != tt.want {
			t.Errorf("%+v: wrong QR eligibility", tt)
		}
		if pickQRAvailable(opts, &bytes.Buffer{}) {
			t.Fatal("redirected output must remain text")
		}
	}
}
