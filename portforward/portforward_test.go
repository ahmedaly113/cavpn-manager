package portforward_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ahmedaly113/cavpn-manager/api"
	"github.com/ahmedaly113/cavpn-manager/portforward"
	"github.com/coreos/go-iptables/iptables"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// Integration tests for portforwarding, not ran in short mode
// Requires an iptables nat chain named PORTFORWARDING in both iptables and ip6tables

var apiFixture = api.cavpnPeerList{
	api.cavpnPeer{
		IPv4:   "10.99.0.1/32",
		IPv6:   "fc00:bbbb:bbbb:bb01::1/128",
		Ports:  []int{4321, 1234},
		Pubkey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))),
	},
}

var rulesFixture = []string{
	"-A PORTFORWARDING -p tcp -m set --match-set PORTFORWARDING_IPV4 dst -m multiport --dports 1234,4321 -j DNAT --to-destination 10.99.0.1",
	"-A PORTFORWARDING -p udp -m set --match-set PORTFORWARDING_IPV4 dst -m multiport --dports 1234,4321 -j DNAT --to-destination 10.99.0.1",
	"-A PORTFORWARDING -p tcp -m set --match-set PORTFORWARDING_IPV6 dst -m multiport --dports 1234,4321 -j DNAT --to-destination fc00:bbbb:bbbb:bb01::1",
	"-A PORTFORWARDING -p udp -m set --match-set PORTFORWARDING_IPV6 dst -m multiport --dports 1234,4321 -j DNAT --to-destination fc00:bbbb:bbbb:bb01::1",
}

const (
	chain     = "PORTFORWARDING"
	ipsetIPv4 = "PORTFORWARDING_IPV4"
	ipsetIPv6 = "PORTFORWARDING_IPV6"
	table     = "nat"
)

func TestPortforward(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests")
	}

	pf, err := portforward.New(chain, ipsetIPv4, ipsetIPv6)
	if err != nil {
		t.Fatal(err)
	}

	ipts := setupIptables(t)

	t.Run("add rules", func(t *testing.T) {
		pf.UpdatePortforwarding(apiFixture)

		rules := getRules(t, ipts)
		if diff := cmp.Diff(rulesFixture, rules, cmpopts.SortSlices(stringCompare)); diff != "" {
			t.Fatalf("unexpected rules (-want +got):\n%s", diff)
		}
	})

	t.Run("remove rules", func(t *testing.T) {
		pf.UpdatePortforwarding(api.cavpnPeerList{})

		rules := getRules(t, ipts)
		if diff := cmp.Diff([]string{}, rules); diff != "" {
			t.Fatalf("unexpected rules (-want +got):\n%s", diff)
		}
	})

	t.Run("add rules for single peer", func(t *testing.T) {
		pf.AddPortforwarding(apiFixture[0])

		rules := getRules(t, ipts)
		if diff := cmp.Diff(rulesFixture, rules, cmpopts.SortSlices(stringCompare)); diff != "" {
			t.Fatalf("unexpected rules (-want +got):\n%s", diff)
		}
	})

	t.Run("remove rules for single peer", func(t *testing.T) {
		pf.RemovePortforwarding(apiFixture[0])

		rules := getRules(t, ipts)
		if diff := cmp.Diff([]string{}, rules); diff != "" {
			t.Fatalf("unexpected rules (-want +got):\n%s", diff)
		}
	})
}

func stringCompare(i string, j string) bool {
	return i < j
}

func getRules(t *testing.T, ipts []*iptables.IPTables) []string {
	t.Helper()

	rules := []string{}
	for _, ipt := range ipts {
		listRules, err := ipt.List(table, chain)
		if err != nil {
			t.Fatal(err)
		}

		if len(listRules) > 0 {
			listRules = listRules[1:]
		}

		for _, rule := range listRules {
			rules = append(rules, rule)
		}
	}

	return rules
}

func setupIptables(t *testing.T) []*iptables.IPTables {
	t.Helper()

	ip4t, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
	if err != nil {
		t.Fatal(err)
	}

	ip6t, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
	if err != nil {
		t.Fatal(err)
	}

	return []*iptables.IPTables{ip4t, ip6t}
}

func TestInvalidChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests")
	}

	_, err := portforward.New("nonexistant", ipsetIPv4, ipsetIPv6)
	if err == nil {
		t.Fatal("no error")
	}
}

func TestInvalidIPSet(t *testing.T) {
	_, err := portforward.New(chain, "nonexistant", "nonexistant")
	if err == nil {
		t.Fatal("no error")
	}
}
