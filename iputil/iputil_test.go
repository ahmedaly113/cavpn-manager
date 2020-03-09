package iputil_test

import (
	"net"
	"testing"

	"github.com/mullvad/wg-manager/iputil"
)

var ipNet = []net.IPNet{
	{IP: net.ParseIP("1.1.1.1"), Mask: net.IPv4Mask(1, 1, 1, 1)},
	{IP: net.ParseIP("2.2.2.2"), Mask: net.IPv4Mask(1, 1, 1, 1)},
}

func TestEqualIPNet(t *testing.T) {
	tests := []struct {
		Name           string
		ExpectedResult bool
		IPNet          []net.IPNet
	}{
		{"different order", true,
			[]net.IPNet{
				{IP: net.ParseIP("2.2.2.2"), Mask: net.IPv4Mask(1, 1, 1, 1)},
				{IP: net.ParseIP("1.1.1.1"), Mask: net.IPv4Mask(1, 1, 1, 1)},
			},
		},
		{"different mask", false,
			[]net.IPNet{
				{IP: net.ParseIP("1.1.1.1"), Mask: net.IPv4Mask(2, 2, 2, 2)},
				{IP: net.ParseIP("2.2.2.2"), Mask: net.IPv4Mask(1, 1, 1, 1)},
			},
		},
		{"different length", false,
			[]net.IPNet{
				{IP: net.ParseIP("1.1.1.1"), Mask: net.IPv4Mask(1, 1, 1, 1)},
			},
		},
		{"different ip", false,
			[]net.IPNet{
				{IP: net.ParseIP("1.1.1.1"), Mask: net.IPv4Mask(1, 1, 1, 1)},
				{IP: net.ParseIP("3.3.3.3"), Mask: net.IPv4Mask(1, 1, 1, 1)},
			},
		},
		{"nil", false, nil},
	}

	for _, test := range tests {
		matches := iputil.EqualIPNet(ipNet, test.IPNet)
		if matches != test.ExpectedResult {
			t.Errorf("%s: got %v, expected %v", test.Name, matches, test.ExpectedResult)
		}
	}
}
