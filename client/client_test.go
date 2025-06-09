package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/charlinchui/galliard/message"
)

func TestHandshake(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		if err := json.NewDecoder(r.Body).Decode(&reqMsgs); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}
		resp := []message.BayeuxMessage{{
			Channel:    "/meta/handshake",
			ClientID:   "test-client-id",
			Successful: boolPtr(true),
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	err := c.Handshake()
	if err != nil {
		t.Fatalf("Expected successful handshake, got: %v", err)
	}
	if c.clientID != "test-client-id" {
		t.Errorf("Expected clientId 'test-client-id', got %q", c.clientID)
	}
}

func TestSubscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		if err := json.NewDecoder(r.Body).Decode(&reqMsgs); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}
		if reqMsgs[0].Channel != "/meta/subscribe" {
			t.Errorf("Expected /meta/subscribe, got %q", reqMsgs[0].Channel)
		}
		if reqMsgs[0].Subscription != "/foo" {
			t.Errorf("Expected subscription to /foo, got %q", reqMsgs[0].Subscription)
		}
		resp := []message.BayeuxMessage{{
			Channel:      "/meta/subscribe",
			Subscription: "/foo",
			Successful:   boolPtr(true),
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	called := false
	handler := func(msg *message.BayeuxMessage) {
		called = true
	}

	err := c.Subscribe("/foo", handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	for _, h := range c.handlers["/foo"] {
		h(&message.BayeuxMessage{Channel: "foo"})
	}
	if !called {
		t.Errorf("Expected handler to be called for /foo")
	}
}

func boolPtr(b bool) *bool { return &b }
