package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/charlinchui/galliard/message"
)

type Client struct {
	serverURL string
	clientID  string
	handlers  map[string][]func(*message.BayeuxMessage)
	mu        sync.Mutex
}

// NewClient creates a new Bayeux client for the given server URL.
func NewClient(serverURL string) *Client {
	return &Client{
		serverURL: serverURL,
		handlers:  make(map[string][]func(*message.BayeuxMessage)),
	}
}

// Handshake performs the Bayeux handshake and stores the clientID.
func (c *Client) Handshake() error {
	reqMsg := message.BayeuxMessage{
		Channel: "/meta/handshake",
	}

	reqBody, err := json.Marshal([]message.BayeuxMessage{reqMsg})
	if err != nil {
		return fmt.Errorf("Error on the handshake Marshal: %w", err)
	}

	resp, err := http.Post(c.serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("Error on the Handshake call: %w", err)
	}

	defer resp.Body.Close()

	var respMsgs []message.BayeuxMessage
	if err := json.NewDecoder(resp.Body).Decode(&respMsgs); err != nil {
		return fmt.Errorf("Error decoding handshake response: %w", err)
	}

	if len(respMsgs) == 0 || respMsgs[0].ClientID == "" {
		return fmt.Errorf("Error on the hanshake: no clientId in response")
	}

	c.clientID = respMsgs[0].ClientID
	return nil
}

// Subscribe subscribes to a channel and registers a callback for messages.
func (c *Client) Subscribe(channel string, handler func(*message.BayeuxMessage)) error {
	c.mu.Lock()
	c.handlers[channel] = append(c.handlers[channel], handler)
	c.mu.Unlock()

	reqMsg := message.BayeuxMessage{
		Channel:      "/meta/subscribe",
		ClientID:     c.clientID,
		Subscription: channel,
	}

	reqBody, err := json.Marshal([]message.BayeuxMessage{reqMsg})
	if err != nil {
		return fmt.Errorf("Error on the request Marshal: %w", err)
	}

	resp, err := http.Post(c.serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("Error on the subscription request: %w", err)
	}

	defer resp.Body.Close()

	var respMsgs []message.BayeuxMessage
	if err := json.NewDecoder(resp.Body).Decode(&respMsgs); err != nil {
		return fmt.Errorf("Error decoding the message: %w", err)
	}

	if len(respMsgs) == 0 || respMsgs[0].Successful == nil || !*respMsgs[0].Successful {
		return fmt.Errorf("Error on the subscription request: %+v", respMsgs)
	}

	return nil
}

//
// // Publish sends a new message to a channel.
// func (c *Client) Publish(channel string, data map[string]interface{}) error
//
// // Connect starts the long-polling loop to receive messages.
// func (c *Client) Connect() error
//
// // Disconnect gracefully disconnects from the server.
// func (c *Client) Disconnect() error
