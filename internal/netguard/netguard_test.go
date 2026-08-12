package netguard

import (
	"context"
	"net"
	"testing"
)

func TestAllowedIPPolicy(t *testing.T) {
	cases := []struct {
		ip           string
		allowPrivate bool
		want         bool
	}{
		{"8.8.8.8", false, true},
		{"10.0.0.1", false, false},
		{"10.0.0.1", true, true},
		{"127.0.0.1", false, false},
		{"127.0.0.1", true, true},
		{"169.254.169.254", false, false},
		{"169.254.169.254", true, false},
		{"0.0.0.0", true, false},
		{"224.0.0.1", true, false},
	}
	for _, tc := range cases {
		if got := allowedIP(net.ParseIP(tc.ip), tc.allowPrivate); got != tc.want {
			t.Fatalf("allowedIP(%s, %v)=%v want %v", tc.ip, tc.allowPrivate, got, tc.want)
		}
	}
}

func TestResolveAllowedLiteralLinkLocalAlwaysDenied(t *testing.T) {
	if _, err := ResolveAllowed(context.Background(), "169.254.169.254", true); err == nil {
		t.Fatal("expected link-local metadata address to remain denied")
	}
}
