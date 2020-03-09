package wireguard_test

import (
	"encoding/base64"
	"net"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/infosum/statsd"
	"github.com/mullvad/wg-manager/api"
	"github.com/mullvad/wg-manager/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Integration tests for wireguard, not ran in short mode
// Requires a wireguard interface named wg0 to be running on the system

const testInterface = "wg0"

var ipv4Net = net.ParseIP("10.99.0.0")
var ipv6Net = net.ParseIP("fc00:bbbb:bbbb:bb01::")

var apiFixture = api.WireguardPeerList{
	api.WireguardPeer{
		IPv4:   "10.99.0.1/32",
		IPv6:   "fc00:bbbb:bbbb:bb01::1/128",
		Ports:  []int{1234, 4321},
		Pubkey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))),
	},
}

var peerFixture = []wgtypes.Peer{{
	PublicKey: wgKey(),
	AllowedIPs: []net.IPNet{
		net.IPNet{
			IP:   net.ParseIP("10.99.0.1"),
			Mask: net.CIDRMask(32, 32),
		},
		net.IPNet{
			IP:   net.ParseIP("fc00:bbbb:bbbb:bb01::1"),
			Mask: net.CIDRMask(128, 128),
		},
	},
	ProtocolVersion: 1,
}}

func wgKey() wgtypes.Key {
	key, _ := wgtypes.NewKey([]byte(strings.Repeat("a", 32)))
	return key
}

func TestWireguard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests")
	}

	metrics, err := statsd.New()
	if err != nil {
		t.Fatal(err)
	}

	client, err := wgctrl.New()
	if err != nil {
		t.Fatal(err)
	}

	defer client.Close()
	defer resetDevice(t, client)

	wg, err := wireguard.New([]string{testInterface}, metrics)
	if err != nil {
		t.Fatal(err)
	}
	defer wg.Close()

	t.Run("add peers", func(t *testing.T) {
		wg.UpdatePeers(apiFixture)

		device, err := client.Device(testInterface)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(peerFixture, device.Peers); diff != "" {
			t.Fatalf("unexpected peers (-want +got):\n%s", diff)
		}
	})

	t.Run("update peer ip", func(t *testing.T) {
		apiFixture[0].IPv4 = "10.99.0.2/32"
		apiFixture[0].IPv6 = "fc00:bbbb:bbbb:bb01::2/128"
		wg.UpdatePeers(apiFixture)

		device, err := client.Device(testInterface)
		if err != nil {
			t.Fatal(err)
		}

		peerFixture[0].AllowedIPs[0].IP = net.ParseIP("10.99.0.2")
		peerFixture[0].AllowedIPs[1].IP = net.ParseIP("fc00:bbbb:bbbb:bb01::2")

		if diff := cmp.Diff(peerFixture, device.Peers); diff != "" {
			t.Fatalf("unexpected peers (-want +got):\n%s", diff)
		}
	})

	t.Run("remove peers", func(t *testing.T) {
		wg.UpdatePeers(api.WireguardPeerList{})

		device, err := client.Device(testInterface)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff([]wgtypes.Peer(nil), device.Peers); diff != "" {
			t.Fatalf("unexpected peers (-want +got):\n%s", diff)
		}
	})

	t.Run("add single peer", func(t *testing.T) {
		wg.AddPeer(apiFixture[0])

		device, err := client.Device(testInterface)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(peerFixture, device.Peers); diff != "" {
			t.Fatalf("unexpected peers (-want +got):\n%s", diff)
		}
	})

	t.Run("remove single peer", func(t *testing.T) {
		wg.RemovePeer(apiFixture[0])

		device, err := client.Device(testInterface)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff([]wgtypes.Peer(nil), device.Peers); diff != "" {
			t.Fatalf("unexpected peers (-want +got):\n%s", diff)
		}
	})
}

func resetDevice(t *testing.T, c *wgctrl.Client) {
	t.Helper()

	cfg := wgtypes.Config{
		ReplacePeers: true,
	}

	if err := c.ConfigureDevice(testInterface, cfg); err != nil {
		t.Fatalf("failed to reset%v", err)
	}
}

func TestInvalidInterface(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration tests")
	}

	interfaceName := "nonexistant"

	_, err := wireguard.New([]string{interfaceName}, nil)
	if err == nil {
		t.Fatal("no error")
	}
}
