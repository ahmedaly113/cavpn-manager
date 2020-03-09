package cavpn

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ahmedaly113/cavpn-manager/api"
	"github.com/infosum/statsd"

	"github.com/ahmedaly113/cavpn-manager/iputil"
	"golang.zx2c4.com/cavpn/cvctrl"
	"golang.zx2c4.com/cavpn/cvctrl/cvtypes"
)

// cavpn is a utility for managing cavpn configuration
type cavpn struct {
	client     *cvctrl.Client
	interfaces []string
	metrics    *statsd.Client
}

// New ensures that the interfaces given are valid, and returns a new cavpn instance
func New(interfaces []string, metrics *statsd.Client) (*cavpn, error) {
	client, err := cvctrl.New()
	if err != nil {
		return nil, err
	}

	for _, i := range interfaces {
		_, err := client.Device(i)
		if err != nil {
			return nil, fmt.Errorf("error getting cavpn interface %s: %s", i, err.Error())
		}
	}

	return &cavpn{
		client:     client,
		interfaces: interfaces,
		metrics:    metrics,
	}, nil
}

// UpdatePeers updates the configuration of the cavpn interfaces to match the given list of peers
func (w *cavpn) UpdatePeers(peers api.cavpnPeerList) {
	peerMap := w.mapPeers(peers)

	var connectedPeers int
	for _, d := range w.interfaces {
		device, err := w.client.Device(d)
		// Log an error, but move on, so that one broken cavpn interface doesn't prevent us from configuring the rest
		if err != nil {
			log.Printf("error connecting to cavpn interface %s: %s", d, err.Error())
			continue
		}

		connectedPeers += countConnectedPeers(device.Peers)

		existingPeerMap := mapExistingPeers(device.Peers)
		cfgPeers := []cvtypes.PeerConfig{}
		resetPeers := []cvtypes.PeerConfig{}

		// Loop through peers from the API
		// Add peers not currently existing in the cavpn config
		// Update peers that exist in the cavpn config but has changed
		for key, allowedIPs := range peerMap {
			existingPeer, ok := existingPeerMap[key]
			if !ok || !iputil.EqualIPNet(allowedIPs, existingPeer.AllowedIPs) {
				cfgPeers = append(cfgPeers, cvtypes.PeerConfig{
					PublicKey:         key,
					ReplaceAllowedIPs: true,
					AllowedIPs:        allowedIPs,
				})
			}
		}

		// Loop through the current peers in the cavpn config
		for key, peer := range existingPeerMap {
			if _, ok := peerMap[key]; !ok {
				// Remove peers that doesn't exist in the API
				cfgPeers = append(cfgPeers, cvtypes.PeerConfig{
					PublicKey: key,
					Remove:    true,
				})
			} else if needsReset(peer) {
				// Remove peers that's previously been active and should be reset to remove data
				cfgPeers = append(cfgPeers, cvtypes.PeerConfig{
					PublicKey: key,
					Remove:    true,
				})

				peerCfg := cvtypes.PeerConfig{
					PublicKey:         key,
					ReplaceAllowedIPs: true,
					AllowedIPs:        peer.AllowedIPs,
				}

				// Copy the preshared key if one is set
				var emptyKey cvtypes.Key
				if peer.PresharedKey != emptyKey {
					// We need to copy the key, or the pointer gets corrupted for some reason
					var copiedKey cvtypes.Key
					copy(copiedKey[:], peer.PresharedKey[:])
					peerCfg.PresharedKey = &copiedKey
				}

				// Re-add the peer later
				resetPeers = append(resetPeers, peerCfg)
			}
		}

		// No changes needed
		if len(cfgPeers) == 0 {
			continue
		}

		// Add new peers, remove deleted peers, and remove peers should be reset
		err = w.client.ConfigureDevice(d, cvtypes.Config{
			Peers: cfgPeers,
		})

		if err != nil {
			log.Printf("error configuring cavpn interface %s: %s", d, err.Error())
			continue
		}

		// No peers to re-add for reset
		if len(resetPeers) == 0 {
			continue
		}

		// Re-add the peers we removed to reset in the previous step
		err = w.client.ConfigureDevice(d, cvtypes.Config{
			Peers: resetPeers,
		})

		if err != nil {
			log.Printf("error configuring cavpn interface %s: %s", d, err.Error())
			continue
		}
	}

	// Send metrics
	w.metrics.Gauge("connected_peers", connectedPeers)
}

// Take the cavpn peers and convert them into a map for easier comparison
func (w *cavpn) mapPeers(peers api.cavpnPeerList) (peerMap map[cvtypes.Key][]net.IPNet) {
	peerMap = make(map[cvtypes.Key][]net.IPNet)

	// Ignore peers with errors, in-case we get bad data from the API
	for _, peer := range peers {
		key, ipv4, ipv6, err := parsePeer(peer)
		if err != nil {
			continue
		}

		peerMap[key] = []net.IPNet{
			*ipv4,
			*ipv6,
		}
	}

	return
}

// Take the existing cavpn peers and convert them into a map for easier comparison
func mapExistingPeers(peers []cvtypes.Peer) (peerMap map[cvtypes.Key]cvtypes.Peer) {
	peerMap = make(map[cvtypes.Key]cvtypes.Peer)

	for _, peer := range peers {
		peerMap[peer.PublicKey] = peer
	}

	return
}

// cavpn sends a handshake roughly every 2 minutes
// So we consider all peers with a handshake within that interval to be connected
const handshakeInterval = time.Minute * 2

// Count the connected cavpn peers
func countConnectedPeers(peers []cvtypes.Peer) (connectedPeers int) {
	for _, peer := range peers {
		if time.Since(peer.LastHandshakeTime) <= handshakeInterval {
			connectedPeers++
		}
	}

	return
}

// A cavpn session can't last for longer then 3 minutes
const inactivityTime = time.Minute * 3

// Whether a peer should be reset or not, to zero out last handshake/bandwidth information
func needsReset(peer cvtypes.Peer) bool {
	if !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) > inactivityTime {
		return true
	}

	return false
}

// AddPeer adds the given peer to the cavpn interfaces, without checking the existing configuration
func (w *cavpn) AddPeer(peer api.cavpnPeer) {
	key, ipv4, ipv6, err := parsePeer(peer)
	if err != nil {
		return
	}

	for _, d := range w.interfaces {
		// Add the peer
		err := w.client.ConfigureDevice(d, cvtypes.Config{
			Peers: []cvtypes.PeerConfig{
				cvtypes.PeerConfig{
					PublicKey:         key,
					ReplaceAllowedIPs: true,
					AllowedIPs: []net.IPNet{
						*ipv4,
						*ipv6,
					},
				},
			},
		})

		if err != nil {
			log.Printf("error configuring cavpn interface %s: %s", d, err.Error())
			continue
		}
	}
}

// RemovePeer removes the given peer from the cavpn interfaces, without checking the existing configuration
func (w *cavpn) RemovePeer(peer api.cavpnPeer) {
	key, _, _, err := parsePeer(peer)
	if err != nil {
		return
	}

	for _, d := range w.interfaces {
		// Remove the peer
		err := w.client.ConfigureDevice(d, cvtypes.Config{
			Peers: []cvtypes.PeerConfig{
				cvtypes.PeerConfig{
					PublicKey: key,
					Remove:    true,
				},
			},
		})

		if err != nil {
			log.Printf("error configuring cavpn interface %s: %s", d, err.Error())
			continue
		}
	}
}

func parsePeer(peer api.cavpnPeer) (key cvtypes.Key, ipv4 *net.IPNet, ipv6 *net.IPNet, err error) {
	key, err = cvtypes.ParseKey(peer.Pubkey)
	if err != nil {
		return
	}

	_, ipv4, err = net.ParseCIDR(peer.IPv4)
	if err != nil {
		return
	}

	_, ipv6, err = net.ParseCIDR(peer.IPv6)
	if err != nil {
		return
	}

	return
}

// Close closes the underlying cavpn client
func (w *cavpn) Close() {
	w.client.Close()
}
