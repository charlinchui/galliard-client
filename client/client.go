package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/charlinchui/galliard/message"
)

type handlerEntry struct {
	id      int
	handler func(*message.BayeuxMessage)
}

// Client implements a Bayeux protocol client for connecting to a Bayeux server.
type Client struct {
	serverURL     string
	clientID      string
	handlers      map[string][]handlerEntry
	handlersMu    sync.RWMutex
	mu            sync.Mutex
	done          chan struct{}
	running       bool
	nextHandlerID int
}

// NewClient creates a new Bayeux client for the given server URL.
func NewClient(serverURL string) *Client {
	return &Client{
		serverURL: serverURL,
		handlers:  make(map[string][]handlerEntry),
		done:      make(chan struct{}),
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
// Returns an unsubscribe function that removes the handler.
func (c *Client) Subscribe(channel string, handler func(*message.BayeuxMessage)) (func(), error) {
	c.handlersMu.Lock()
	c.nextHandlerID++
	entry := handlerEntry{id: c.nextHandlerID, handler: handler}
	c.handlers[channel] = append(c.handlers[channel], entry)
	c.handlersMu.Unlock()

	reqMsg := message.BayeuxMessage{
		Channel:      "/meta/subscribe",
		ClientID:     c.clientID,
		Subscription: channel,
	}

	reqBody, err := json.Marshal([]message.BayeuxMessage{reqMsg})
	if err != nil {
		return nil, fmt.Errorf("Error during request marshal: %w", err)
	}

	resp, err := http.Post(c.serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("Error on the subscription request: %w", err)
	}
	defer resp.Body.Close()

	var respMsgs []message.BayeuxMessage
	if err := json.NewDecoder(resp.Body).Decode(&respMsgs); err != nil {
		return nil, fmt.Errorf("Error decoding the message: %w", err)
	}

	if len(respMsgs) == 0 || respMsgs[0].Successful == nil || !*respMsgs[0].Successful {
		return nil, fmt.Errorf("Error on the subscription request: %+v", respMsgs)
	}

	unsubscribe := func() {
		c.handlersMu.Lock()
		defer c.handlersMu.Unlock()
		handlers := c.handlers[channel]
		newHandlers := handlers[:0]
		for _, h := range handlers {
			if h.id != entry.id {
				newHandlers = append(newHandlers, h)
			}
		}
		c.handlers[channel] = newHandlers
	}
	return unsubscribe, nil
}

// Publish sends a new message to a channel.
func (c *Client) Publish(channel string, data map[string]interface{}) error {
	reqMsg := message.BayeuxMessage{
		Channel:  channel,
		ClientID: c.clientID,
		Data:     data,
	}

	reqBody, err := json.Marshal([]message.BayeuxMessage{reqMsg})
	if err != nil {
		return fmt.Errorf("Error on during request marshal: %w", err)
	}

	resp, err := http.Post(c.serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("Error on the publish request: %w", err)
	}
	defer resp.Body.Close()

	var respMsgs []message.BayeuxMessage
	if err := json.NewDecoder(resp.Body).Decode(&respMsgs); err != nil {
		return fmt.Errorf("Error decoding the message: %w", err)
	}

	if len(respMsgs) == 0 || respMsgs[0].Successful == nil || !*respMsgs[0].Successful {
		return fmt.Errorf("Error on the publish request: %+v", respMsgs)
	}

	return nil
}

// Connect starts the long-polling loop to receive messages.
func (c *Client) Connect() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("Error: Connect loop already running")
	}
	c.running = true
	c.mu.Unlock()

	go func() {
		for {
			select {
			case <-c.done:
				return
			default:
				err := c.connectOnce()
				if err != nil {
					time.Sleep(1 * time.Second)
				}
			}
		}
	}()

	return nil
}

func (c *Client) connectOnce() error {
	reqMsg := message.BayeuxMessage{
		Channel:  "/meta/connect",
		ClientID: c.clientID,
	}

	reqBody, err := json.Marshal([]message.BayeuxMessage{reqMsg})
	if err != nil {
		return err
	}

	resp, err := http.Post(c.serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var respMsgs []message.BayeuxMessage
	if err := json.NewDecoder(resp.Body).Decode(&respMsgs); err != nil {
		return err
	}

	for _, msg := range respMsgs {
		c.handlersMu.RLock()
		handlers := c.handlers[msg.Channel]
		c.handlersMu.RUnlock()
		for _, entry := range handlers {
			go func(h func(*message.BayeuxMessage), msg *message.BayeuxMessage) {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("%+v", r)
					}
				}()
				h(msg)
			}(entry.handler, &msg)
		}
	}

	return nil
}

// Disconnect gracefully disconnects from the server and stops the connect loop.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	if c.running {
		close(c.done)
		c.running = false
		c.done = make(chan struct{})
	}
	c.mu.Unlock()

	reqMsg := message.BayeuxMessage{
		Channel:  "/meta/disconnect",
		ClientID: c.clientID,
	}

	reqBody, err := json.Marshal([]message.BayeuxMessage{reqMsg})
	if err != nil {
		return fmt.Errorf("Error disconnecting: %w", err)
	}

	resp, err := http.Post(c.serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("Error on the disconnect request: %w", err)
	}
	defer resp.Body.Close()

	var respMsgs []message.BayeuxMessage
	if err := json.NewDecoder(resp.Body).Decode(&respMsgs); err != nil {
		return fmt.Errorf("Error decoding disconnect response: %w", err)
	}

	if len(respMsgs) == 0 || respMsgs[0].Successful == nil || !*respMsgs[0].Successful {
		return fmt.Errorf("Error disconnecting from channel %+v", respMsgs)
	}

	return nil
}
