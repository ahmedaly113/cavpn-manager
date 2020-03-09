package subscriber

import (
	"context"
	"encoding/base64"
	"log"
	"net/http"
	"time"

	"github.com/infosum/statsd"
	"github.com/mullvad/wg-manager/api"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// Subscriber is a utility for receiving wireguard key events from a message-queue server
type Subscriber struct {
	Username string
	Password string
	BaseURL  string
	Channel  string
	Metrics  *statsd.Client
}

// WireguardEvent is a wireguard key event
type WireguardEvent struct {
	Action string            `json:"action"`
	Peer   api.WireguardPeer `json:"peer"`
}

const subProtocol = "message-queue-v1"

// Subscribe establishes a websocket connection for a message-queue channel, and emits messages on the given channel
func (s *Subscriber) Subscribe(ctx context.Context, channel chan<- WireguardEvent) error {
	err := s.connect(ctx, channel)

	if err != nil {
		return err
	}

	return nil
}

func (s *Subscriber) connect(ctx context.Context, channel chan<- WireguardEvent) error {
	header := http.Header{}

	if s.Username != "" && s.Password != "" {
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(s.Username+":"+s.Password)))
	}

	conn, _, err := websocket.Dial(ctx, s.BaseURL+"/channel/"+s.Channel, &websocket.DialOptions{
		Subprotocols: []string{subProtocol},
		HTTPHeader:   header,
	})

	if err != nil {
		return err
	}

	go s.read(ctx, channel, conn)

	return nil
}

func (s *Subscriber) read(ctx context.Context, channel chan<- WireguardEvent, conn *websocket.Conn) {
	for {
		v := WireguardEvent{}
		err := wsjson.Read(ctx, conn, &v)
		if err != nil {
			log.Println("error reading from websocket, reconnecting", err)
			s.Metrics.Increment("websocket_error")

			// Make sure the connection is closed
			conn.Close(websocket.StatusInternalError, "")

			// Start attempting to reconnect
			go s.reconnect(ctx, channel)

			return
		}

		channel <- v
	}
}

func (s *Subscriber) reconnect(ctx context.Context, channel chan<- WireguardEvent) {
	// Sleep
	time.Sleep(time.Second)

	// Attempt to create a new connection
	err := s.connect(ctx, channel)
	if err != nil {
		s.Metrics.Increment("websocket_reconnect_error")
		go s.reconnect(ctx, channel)
	} else {
		log.Println("successfully reconnected to websocket")
		s.Metrics.Increment("websocket_reconnect_success")
	}
}
