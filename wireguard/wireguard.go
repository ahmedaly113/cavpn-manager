package wireguard

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/infosum/statsd"
	"github.com/mullvad/wg-manager/api"

	"github.com/mullvad/wg-manager/iputil"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Wireguard is a utility for managing wireguard configuration
type Wireguard struct {
	client     *wgctrl.Client
	interfaces []string
	metrics    *statsd.Client
}

// New ensures that the interfaces given are valid, and returns a new Wireguard instance
func New(interfaces []string, metrics *statsd.Client) (*Wireguard, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, err
	}

	for _, i := range interfaces {
		_, err := client.Device(i)
		if err != nil {
			return nil, fmt.Errorf("error getting wireguard interface %s: %s", i, err.Error())
		}
	}

	return &Wireguard{
		client:     client,
		interfaces: interfaces,
		metrics:    metrics,
	}, nil
}

// UpdatePeers updates the configuration of the wireguard interfaces to match the given list of peers
func (w *Wireguard) UpdatePeers(peers api.WireguardPeerList) {
	peerMap := w.mapPeers(peers)

	var connectedPeers int
	for _, d := range w.interfaces {
		device, err := w.client.Device(d)
		// Log an error, but move on, so that one broken wireguard interface doesn't prevent us from configuring the rest
		if err != nil {
			log.Printf("error connecting to wireguard interface %s: %s", d, err.Error())
			continue
		}

		connectedPeers += countConnectedPeers(device.Peers)

		existingPeerMap := mapExistingPeers(device.Peers)
		cfgPeers := []wgtypes.PeerConfig{}
		resetPeers := []wgtypes.PeerConfig{}

		// Loop through peers from the API
		// Add peers not currently existing in the wireguard config
		// Update peers that exist in the wireguard config but has changed
		for key, allowedIPs := range peerMap {
			existingPeer, ok := existingPeerMap[key]
			if !ok || !iputil.EqualIPNet(allowedIPs, existingPeer.AllowedIPs) {
				cfgPeers = append(cfgPeers, wgtypes.PeerConfig{
					PublicKey:         key,
					ReplaceAllowedIPs: true,
					AllowedIPs:        allowedIPs,
				})
			}
		}

		// Loop through the current peers in the wireguard config
		for key, peer := range existingPeerMap {
			if _, ok := peerMap[key]; !ok {
				// Remove peers that doesn't exist in the API
				cfgPeers = append(cfgPeers, wgtypes.PeerConfig{
					PublicKey: key,
					Remove:    true,
				})
			} else if needsReset(peer) {
				// Remove peers that's previously been active and should be reset to remove data
				cfgPeers = append(cfgPeers, wgtypes.PeerConfig{
					PublicKey: key,
					Remove:    true,
				})

				peerCfg := wgtypes.PeerConfig{
					PublicKey:         key,
					ReplaceAllowedIPs: true,
					AllowedIPs:        peer.AllowedIPs,
				}

				// Copy the preshared key if one is set
				var emptyKey wgtypes.Key
				if peer.PresharedKey != emptyKey {
					// We need to copy the key, or the pointer gets corrupted for some reason
					var copiedKey wgtypes.Key
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
		err = w.client.ConfigureDevice(d, wgtypes.Config{
			Peers: cfgPeers,
		})

		if err != nil {
			log.Printf("error configuring wireguard interface %s: %s", d, err.Error())
			continue
		}

		// No peers to re-add for reset
		if len(resetPeers) == 0 {
			continue
		}

		// Re-add the peers we removed to reset in the previous step
		err = w.client.ConfigureDevice(d, wgtypes.Config{
			Peers: resetPeers,
		})

		if err != nil {
			log.Printf("error configuring wireguard interface %s: %s", d, err.Error())
			continue
		}
	}

	// Send metrics
	w.metrics.Gauge("connected_peers", connectedPeers)
}

// Take the wireguard peers and convert them into a map for easier comparison
func (w *Wireguard) mapPeers(peers api.WireguardPeerList) (peerMap map[wgtypes.Key][]net.IPNet) {
	peerMap = make(map[wgtypes.Key][]net.IPNet)

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

// Take the existing wireguard peers and convert them into a map for easier comparison
func mapExistingPeers(peers []wgtypes.Peer) (peerMap map[wgtypes.Key]wgtypes.Peer) {
	peerMap = make(map[wgtypes.Key]wgtypes.Peer)

	for _, peer := range peers {
		peerMap[peer.PublicKey] = peer
	}

	return
}

// Wireguard sends a handshake roughly every 2 minutes
// So we consider all peers with a handshake within that interval to be connected
const handshakeInterval = time.Minute * 2

// Count the connected wireguard peers
func countConnectedPeers(peers []wgtypes.Peer) (connectedPeers int) {
	for _, peer := range peers {
		if time.Since(peer.LastHandshakeTime) <= handshakeInterval {
			connectedPeers++
		}
	}

	return
}

// A wireguard session can't last for longer then 3 minutes
const inactivityTime = time.Minute * 3

// Whether a peer should be reset or not, to zero out last handshake/bandwidth information
func needsReset(peer wgtypes.Peer) bool {
	if !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) > inactivityTime {
		return true
	}

	return false
}

// AddPeer adds the given peer to the wireguard interfaces, without checking the existing configuration
func (w *Wireguard) AddPeer(peer api.WireguardPeer) {
	key, ipv4, ipv6, err := parsePeer(peer)
	if err != nil {
		return
	}

	for _, d := range w.interfaces {
		// Add the peer
		err := w.client.ConfigureDevice(d, wgtypes.Config{
			Peers: []wgtypes.PeerConfig{
				wgtypes.PeerConfig{
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
			log.Printf("error configuring wireguard interface %s: %s", d, err.Error())
			continue
		}
	}
}

// RemovePeer removes the given peer from the wireguard interfaces, without checking the existing configuration
func (w *Wireguard) RemovePeer(peer api.WireguardPeer) {
	key, _, _, err := parsePeer(peer)
	if err != nil {
		return
	}

	for _, d := range w.interfaces {
		// Remove the peer
		err := w.client.ConfigureDevice(d, wgtypes.Config{
			Peers: []wgtypes.PeerConfig{
				wgtypes.PeerConfig{
					PublicKey: key,
					Remove:    true,
				},
			},
		})

		if err != nil {
			log.Printf("error configuring wireguard interface %s: %s", d, err.Error())
			continue
		}
	}
}

func parsePeer(peer api.WireguardPeer) (key wgtypes.Key, ipv4 *net.IPNet, ipv6 *net.IPNet, err error) {
	key, err = wgtypes.ParseKey(peer.Pubkey)
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

// Close closes the underlying wireguard client
func (w *Wireguard) Close() {
	w.client.Close()
}
