package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func PublicIP(ip net.IP) bool {
	return allowedIP(ip, false)
}

func allowedIP(ip net.IP, allowPrivate bool) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip.IsLoopback() {
		return allowPrivate
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	if ip.IsPrivate() {
		return allowPrivate
	}
	return true
}

func ResolveAllowed(ctx context.Context, host string, allowPrivate bool) ([]net.IP, error) {
	if parsed := net.ParseIP(host); parsed != nil {
		if !allowedIP(parsed, allowPrivate) {
			return nil, errors.New("private/local or non-routable network address denied")
		}
		return []net.IP{parsed}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var out []net.IP
	for _, a := range addrs {
		if allowedIP(a.IP, allowPrivate) {
			out = append(out, a.IP)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("hostname resolved only to denied private/local addresses")
	}
	return out, nil
}

func DialContext(allowPrivate bool) func(context.Context, string, string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := ResolveAllowed(ctx, host, allowPrivate)
		if err != nil {
			return nil, err
		}
		var last error
		for _, ip := range ips {
			conn, er := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if er == nil {
				return conn, nil
			}
			last = er
		}
		if last == nil {
			last = fmt.Errorf("no allowed address for %s", host)
		}
		return nil, last
	}
}

func Client(timeout time.Duration, allowPrivate bool) *http.Client {
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           DialContext(allowPrivate),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}
