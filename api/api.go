package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// API is a utility for communicating with the Mullvad API
type API struct {
	Username string
	Password string
	BaseURL  string
	Client   *http.Client
}

// WireguardPeerList is a list of Wireguard peers
type WireguardPeerList []WireguardPeer

// WireguardPeer is a wireguard peer
type WireguardPeer struct {
	IPv4   string `json:"ipv4"`
	IPv6   string `json:"ipv6"`
	Ports  []int  `json:"ports"`
	Pubkey string `json:"pubkey"`
}

// GetWireguardPeers fetches a list of wireguard peers from the API and returns it
func (a *API) GetWireguardPeers() (WireguardPeerList, error) {
	req, err := http.NewRequest("GET", a.BaseURL+"/wg/active-pubkeys/v2/", nil)
	if err != nil {
		return WireguardPeerList{}, err
	}

	if a.Username != "" && a.Password != "" {
		req.SetBasicAuth(a.Username, a.Password)
	}

	response, err := a.Client.Do(req)
	if err != nil {
		return WireguardPeerList{}, err
	}

	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return WireguardPeerList{}, err
	}

	var decodedResponse WireguardPeerList
	err = json.Unmarshal(body, &decodedResponse)
	if err != nil {
		return WireguardPeerList{}, fmt.Errorf("error decoding wireguard peers")
	}

	return decodedResponse, nil
}
