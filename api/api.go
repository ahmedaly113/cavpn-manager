package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// API is a utility for communicating with the ahmedaly113 API
type API struct {
	Username string
	Password string
	BaseURL  string
	Client   *http.Client
}

// cavpnPeerList is a list of cavpn peers
type cavpnPeerList []cavpnPeer

// cavpnPeer is a cavpn peer
type cavpnPeer struct {
	IPv4   string `json:"ipv4"`
	IPv6   string `json:"ipv6"`
	Ports  []int  `json:"ports"`
	Pubkey string `json:"pubkey"`
}

// GetcavpnPeers fetches a list of cavpn peers from the API and returns it
func (a *API) GetcavpnPeers() (cavpnPeerList, error) {
	req, err := http.NewRequest("GET", a.BaseURL+"/cv/active-pubkeys/v2/", nil)
	if err != nil {
		return cavpnPeerList{}, err
	}

	if a.Username != "" && a.Password != "" {
		req.SetBasicAuth(a.Username, a.Password)
	}

	response, err := a.Client.Do(req)
	if err != nil {
		return cavpnPeerList{}, err
	}

	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return cavpnPeerList{}, err
	}

	var decodedResponse cavpnPeerList
	err = json.Unmarshal(body, &decodedResponse)
	if err != nil {
		return cavpnPeerList{}, fmt.Errorf("error decoding cavpn peers")
	}

	return decodedResponse, nil
}
