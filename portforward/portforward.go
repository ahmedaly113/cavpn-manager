package portforward

import (
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/coreos/go-iptables/iptables"
	"github.com/digineo/go-ipset/v2"
	"github.com/mdlayher/netlink"
	"github.com/mullvad/wg-manager/api"
	"github.com/ti-mo/netfilter"
)

// Portforward is a utility for managing portforwarding
type Portforward struct {
	iptables  *iptables.IPTables
	ip6tables *iptables.IPTables
	chain     string
	ipsetIPv4 string
	ipsetIPv6 string
}

// Iptables table to operate against
const table = "nat"

// New validates the addresses, ensures that the iptables portforwarding chain exists, and returns a new Portforward instance
func New(chain string, ipsetTableIPv4 string, ipsetTableIPv6 string) (*Portforward, error) {
	ipt, err := newIPTables(chain, iptables.ProtocolIPv4)
	if err != nil {
		return nil, err
	}

	ip6t, err := newIPTables(chain, iptables.ProtocolIPv6)
	if err != nil {
		return nil, err
	}

	err = validateIPSet(ipsetTableIPv4)
	if err != nil {
		return nil, err
	}

	err = validateIPSet(ipsetTableIPv6)
	if err != nil {
		return nil, err
	}

	return &Portforward{
		iptables:  ipt,
		ip6tables: ip6t,
		chain:     chain,
		ipsetIPv4: ipsetTableIPv4,
		ipsetIPv6: ipsetTableIPv6,
	}, nil
}

func newIPTables(chain string, protocol iptables.Protocol) (*iptables.IPTables, error) {
	ipt, err := iptables.NewWithProtocol(protocol)
	if err != nil {
		return nil, err
	}

	exists, err := chainExists(chain, ipt)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, fmt.Errorf("an iptables chain named %s does not exist", chain)
	}

	return ipt, nil
}

func chainExists(chain string, ipt *iptables.IPTables) (bool, error) {
	chains, err := ipt.ListChains("nat")

	if err != nil {
		return false, err
	}

	for _, c := range chains {
		if c == chain {
			return true, nil
		}
	}

	return false, nil
}

func validateIPSet(name string) error {
	conn, err := ipset.Dial(netfilter.ProtoUnspec, &netlink.Config{})
	if err != nil {
		return err
	}

	ipsets, err := conn.ListAll()

	for _, p := range ipsets {
		if p.Name.Get() == name {
			return nil
		}
	}

	return fmt.Errorf("an ipset named %s does not exist", name)
}

// UpdatePortforwarding updates the iptables rules for portforwarding to match the given list of peers
func (p *Portforward) UpdatePortforwarding(peers api.WireguardPeerList) {
	rules := make(map[string]iptables.Protocol)
	for _, peer := range peers {
		if len(peer.Ports) < 1 {
			continue
		}

		p.createPeerRules(peer, rules)
	}

	currentRules, err := p.getCurrentRules()
	if err != nil {
		log.Printf("error getting current iptables rules %s", err.Error())
		return
	}

	// Add new portforwarding rules
	for rule, protocol := range rules {
		if _, ok := currentRules[rule]; !ok {
			ipt := p.iptables
			if protocol == iptables.ProtocolIPv6 {
				ipt = p.ip6tables
			}

			err := ipt.Append(table, p.chain, strings.Split(rule, " ")...)
			if err != nil {
				log.Printf("error adding iptables rule")
				continue
			}
		}
	}

	// Remove old portforwarding rules
	for rule, protocol := range currentRules {
		if _, ok := rules[rule]; !ok {
			ipt := p.iptables
			if protocol == iptables.ProtocolIPv6 {
				ipt = p.ip6tables
			}

			err := ipt.Delete(table, p.chain, strings.Split(rule, " ")...)
			if err != nil {
				log.Printf("error deleting iptables rule")
				continue
			}
		}
	}
}

// AddPortforwarding tries to add portforwarding rules for a peer without checking existing ones
func (p *Portforward) AddPortforwarding(peer api.WireguardPeer) {
	if len(peer.Ports) < 1 {
		return
	}

	rules := make(map[string]iptables.Protocol)
	p.createPeerRules(peer, rules)

	// Add new portforwarding rules
	for rule, protocol := range rules {
		ipt := p.iptables
		if protocol == iptables.ProtocolIPv6 {
			ipt = p.ip6tables
		}

		err := ipt.Append(table, p.chain, strings.Split(rule, " ")...)
		if err != nil {
			log.Printf("error adding iptables rule")
			continue
		}
	}

	return
}

// RemovePortforwarding tries to remove portforwarding rules for a peer without checking existing ones
func (p *Portforward) RemovePortforwarding(peer api.WireguardPeer) {
	if len(peer.Ports) < 1 {
		return
	}

	rules := make(map[string]iptables.Protocol)
	p.createPeerRules(peer, rules)

	// Remove old portforwarding rules
	for rule, protocol := range rules {
		ipt := p.iptables
		if protocol == iptables.ProtocolIPv6 {
			ipt = p.ip6tables
		}

		err := ipt.Delete(table, p.chain, strings.Split(rule, " ")...)
		if err != nil {
			log.Printf("error deleting iptables rule")
			continue
		}
	}
}

func (p *Portforward) createPeerRules(peer api.WireguardPeer, rules map[string]iptables.Protocol) {
	// Ignore ip's with errors, in-case we get bad data from the API
	ipv4, _, err := net.ParseCIDR(peer.IPv4)
	if err != nil {
		return
	}

	tcpRule := fmt.Sprintf("-p tcp -m set --match-set %s dst -m multiport --dports %s -j DNAT --to-destination %s", p.ipsetIPv4, getPortsString(peer.Ports), ipv4)
	udpRule := fmt.Sprintf("-p udp -m set --match-set %s dst -m multiport --dports %s -j DNAT --to-destination %s", p.ipsetIPv4, getPortsString(peer.Ports), ipv4)
	rules[tcpRule] = iptables.ProtocolIPv4
	rules[udpRule] = iptables.ProtocolIPv4

	ipv6, _, err := net.ParseCIDR(peer.IPv6)
	if err != nil {
		return
	}

	tcpRule = fmt.Sprintf("-p tcp -m set --match-set %s dst -m multiport --dports %s -j DNAT --to-destination %s", p.ipsetIPv6, getPortsString(peer.Ports), ipv6)
	udpRule = fmt.Sprintf("-p udp -m set --match-set %s dst -m multiport --dports %s -j DNAT --to-destination %s", p.ipsetIPv6, getPortsString(peer.Ports), ipv6)
	rules[tcpRule] = iptables.ProtocolIPv6
	rules[udpRule] = iptables.ProtocolIPv6
}

func getPortsString(ports []int) string {
	sort.Ints(ports)

	slice := make([]string, len(ports))
	for i, v := range ports {
		slice[i] = strconv.Itoa(v)
	}

	return strings.Join(slice, ",")
}

func (p *Portforward) getCurrentRules() (map[string]iptables.Protocol, error) {
	rules := make(map[string]iptables.Protocol)

	ipv4Rules, err := p.iptables.List("nat", p.chain)
	if err != nil {
		return nil, err
	}

	for _, rule := range p.filterRules(ipv4Rules) {
		rules[rule] = iptables.ProtocolIPv4
	}

	ipv6Rules, err := p.ip6tables.List("nat", p.chain)
	if err != nil {
		return nil, err
	}

	for _, rule := range p.filterRules(ipv6Rules) {
		rules[rule] = iptables.ProtocolIPv6
	}

	return rules, nil
}

func (p *Portforward) filterRules(rules []string) []string {
	// Remove the first entry as it's the rule for creating the chain
	if len(rules) > 0 {
		rules = rules[1:]
	}

	var filteredRules []string
	for _, rule := range rules {
		// Remove the chain name
		rule = strings.TrimPrefix(rule, fmt.Sprintf("-A %s ", p.chain))

		// Remove the ip masks
		rule = strings.Replace(rule, "/32", "", -1)
		rule = strings.Replace(rule, "/128", "", -1)

		filteredRules = append(filteredRules, rule)
	}

	return filteredRules
}
