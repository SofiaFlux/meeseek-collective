package teb

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestEgressDialPinsOneValidatedDNSAnswer(t *testing.T) {
	policy := EgressPolicy{AllowedHosts: map[string]struct{}{"api.example.test": {}}}
	lookups := 0
	var dialed string
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		if lookups > 1 {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("93.184.215.14")}, nil
	}
	dial := func(_ context.Context, _, target string) (net.Conn, error) {
		dialed = target
		client, server := net.Pipe()
		server.Close()
		return client, nil
	}
	conn, err := dialAllowedTarget(context.Background(), policy, "api.example.test:443", lookup, dial)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if lookups != 1 || dialed != "93.184.215.14:443" {
		t.Fatalf("lookups=%d dialed=%q; target must be validated IP", lookups, dialed)
	}
}

func TestEgressDialRejectsAnyNonPublicDNSAnswer(t *testing.T) {
	policy := EgressPolicy{AllowedHosts: map[string]struct{}{"api.example.test": {}}}
	for _, address := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.64.0.1", "192.0.2.1", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1"} {
		t.Run(address, func(t *testing.T) {
			dialed := false
			lookup := func(context.Context, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("93.184.215.14"), netip.MustParseAddr(address)}, nil
			}
			dial := func(context.Context, string, string) (net.Conn, error) {
				dialed = true
				return nil, errors.New("unexpected dial")
			}
			if _, err := dialAllowedTarget(context.Background(), policy, "api.example.test:443", lookup, dial); !errors.Is(err, errEgressDenied) {
				t.Fatalf("DNS answer %s error = %v, want denial", address, err)
			}
			if dialed {
				t.Fatal("dialed after non-public DNS answer")
			}
		})
	}
}

func TestEgressDialAllowsExactLiteralWithoutDNS(t *testing.T) {
	policy := EgressPolicy{AllowedHosts: map[string]struct{}{"127.0.0.1": {}}}
	lookup := func(context.Context, string) ([]netip.Addr, error) { t.Fatal("literal IP used DNS"); return nil, nil }
	var dialed string
	dial := func(_ context.Context, _, target string) (net.Conn, error) {
		dialed = target
		client, server := net.Pipe()
		server.Close()
		return client, nil
	}
	conn, err := dialAllowedTarget(context.Background(), policy, "127.0.0.1:8080", lookup, dial)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if dialed != "127.0.0.1:8080" {
		t.Fatalf("literal target = %q", dialed)
	}
}
