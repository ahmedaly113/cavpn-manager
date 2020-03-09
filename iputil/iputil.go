package iputil

import (
	"net"
	"sort"
)

// EqualIPNet checks whether two slices of IPNet are equal
func EqualIPNet(a []net.IPNet, b []net.IPNet) bool {
	if (a == nil) != (b == nil) {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	sort.Slice(a, compareIPNet(a))
	sort.Slice(b, compareIPNet(b))

	for i := range a {
		if a[i].String() != b[i].String() {
			return false
		}
	}

	return true
}

func compareIPNet(ips []net.IPNet) func(i int, j int) bool {
	return func(i int, j int) bool {
		return ips[i].String() < ips[j].String()
	}
}
