package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/charlinchui/galliard/message"
)

func boolPtr(b bool) *bool { return &b }

func TestNewClient(t *testing.T) {
	c := NewClient("http://example.com/bayeux")
	if c.serverURL != "http://example.com/bayeux" {
		t.Errorf("Expected serverURL to be 'http://example.com/bayeux', got %q", c.serverURL)
	}
	if c.handlers == nil {
		t.Errorf("Expected handlers map to be initialized")
	}
	if c.done == nil {
		t.Errorf("Expected done channel to be initialized")
	}
}

func TestHandshake(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		if len(reqMsgs) == 0 || reqMsgs[0].Channel != "/meta/handshake" {
			t.Errorf("Expected handshake request, got %+v", reqMsgs)
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
		t.Fatalf("Handshake failed: %v", err)
	}
	if c.clientID != "test-client-id" {
		t.Errorf("Expected clientID 'test-client-id', got %q", c.clientID)
	}
}

func TestHandshakeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []message.BayeuxMessage{{
			Channel:    "/meta/handshake",
			Successful: boolPtr(false),
			Error:      "Handshake failed",
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	err := c.Handshake()
	if err == nil {
		t.Errorf("Expected handshake to fail")
	}
}

func TestSubscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		if len(reqMsgs) == 0 || reqMsgs[0].Channel != "/meta/subscribe" {
			t.Errorf("Expected subscribe request, got %+v", reqMsgs)
		}

		if reqMsgs[0].Subscription != "/foo" {
			t.Errorf("Expected subscription to '/foo', got %q", reqMsgs[0].Subscription)
		}

		resp := []message.BayeuxMessage{{
			Channel:      "/meta/subscribe",
			Successful:   boolPtr(true),
			Subscription: "/foo",
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	called := false
	handler := func(msg *message.BayeuxMessage) { called = true }

	unsubscribe, err := c.Subscribe("/foo", handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	c.handlersMu.RLock()
	handlers, exists := c.handlers["/foo"]
	c.handlersMu.RUnlock()

	if !exists || len(handlers) == 0 {
		t.Fatalf("Expected handler to be registered")
	}

	handlers[0].handler(&message.BayeuxMessage{})
	if !called {
		t.Errorf("Expected handler to be called")
	}

	unsubscribe()

	c.handlersMu.RLock()
	handlers, exists = c.handlers["/foo"]
	c.handlersMu.RUnlock()

	if len(handlers) > 0 {
		for _, h := range handlers {
			if h.id == handlers[0].id {
				t.Errorf("Expected handler to be removed")
			}
		}
	}
}

func TestSubscribeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []message.BayeuxMessage{{
			Channel:      "/meta/subscribe",
			Successful:   boolPtr(false),
			Error:        "Subscription failed",
			Subscription: "/foo",
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	_, err := c.Subscribe("/foo", func(msg *message.BayeuxMessage) {})
	if err == nil {
		t.Errorf("Expected subscribe to fail")
	}
}

func TestMultipleSubscriptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		resp := []message.BayeuxMessage{{
			Channel:      "/meta/subscribe",
			Successful:   boolPtr(true),
			Subscription: reqMsgs[0].Subscription,
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	var called1, called2 bool
	handler1 := func(msg *message.BayeuxMessage) { called1 = true }
	handler2 := func(msg *message.BayeuxMessage) { called2 = true }

	unsub1, err := c.Subscribe("/foo", handler1)
	if err != nil {
		t.Fatalf("First subscribe failed: %v", err)
	}

	_, err = c.Subscribe("/foo", handler2)
	if err != nil {
		t.Fatalf("Second subscribe failed: %v", err)
	}

	c.handlersMu.RLock()
	handlers, exists := c.handlers["/foo"]
	c.handlersMu.RUnlock()

	if !exists || len(handlers) != 2 {
		t.Fatalf("Expected 2 handlers, got %d", len(handlers))
	}

	for _, h := range handlers {
		h.handler(&message.BayeuxMessage{})
	}

	if !called1 || !called2 {
		t.Errorf("Expected both handlers to be called")
	}

	called1, called2 = false, false
	unsub1()

	c.handlersMu.RLock()
	handlers, exists = c.handlers["/foo"]
	c.handlersMu.RUnlock()

	if !exists || len(handlers) != 1 {
		t.Fatalf("Expected 1 handler after unsubscribe, got %d", len(handlers))
	}

	for _, h := range handlers {
		h.handler(&message.BayeuxMessage{})
	}

	if called1 {
		t.Errorf("Handler1 should be unsubscribed")
	}
	if !called2 {
		t.Errorf("Handler2 should still be called")
	}
}

func TestUnsubscribeTwice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		resp := []message.BayeuxMessage{{
			Channel:      "/meta/subscribe",
			Successful:   boolPtr(true),
			Subscription: reqMsgs[0].Subscription,
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	handler := func(msg *message.BayeuxMessage) {}
	unsubscribe, err := c.Subscribe("/foo", handler)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	unsubscribe()

	unsubscribe()

	c.handlersMu.RLock()
	handlers, exists := c.handlers["/foo"]
	c.handlersMu.RUnlock()

	if exists && len(handlers) > 0 {
		t.Errorf("Expected no handlers after unsubscribe")
	}
}

func TestPublish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		if len(reqMsgs) == 0 || reqMsgs[0].Channel != "/foo" {
			t.Errorf("Expected publish to '/foo', got %+v", reqMsgs)
		}

		if reqMsgs[0].Data == nil || reqMsgs[0].Data["msg"] != "hello" {
			t.Errorf("Expected data {msg: hello}, got %+v", reqMsgs[0].Data)
		}

		resp := []message.BayeuxMessage{{
			Channel:    "/foo",
			Successful: boolPtr(true),
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	err := c.Publish("/foo", map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
}

func TestPublishError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := []message.BayeuxMessage{{
			Channel:    "/foo",
			Successful: boolPtr(false),
			Error:      "Publish failed",
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	err := c.Publish("/foo", map[string]interface{}{"msg": "hello"})
	if err == nil {
		t.Errorf("Expected publish to fail")
	}
}

func TestConnect(t *testing.T) {
	messageReceived := make(chan bool)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		resp := []message.BayeuxMessage{
			{
				Channel:    "/meta/connect",
				Successful: boolPtr(true),
				ClientID:   "test-client-id",
			},
			{
				Channel: "/foo",
				Data:    map[string]interface{}{"msg": "hello"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	c.Subscribe("/foo", func(msg *message.BayeuxMessage) {
		if msg.Data["msg"] == "hello" {
			messageReceived <- true
		}
	})

	err := c.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	select {
	case <-messageReceived:
	case <-time.After(2 * time.Second):
		t.Errorf("Message not received within timeout")
	}

	c.Disconnect()
}

func TestDisconnect(t *testing.T) {
	disconnectCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		if len(reqMsgs) > 0 && reqMsgs[0].Channel == "/meta/disconnect" {
			disconnectCalled = true
		}

		resp := []message.BayeuxMessage{{
			Channel:    "/meta/disconnect",
			Successful: boolPtr(true),
			ClientID:   "test-client-id",
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	err := c.Disconnect()
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	if !disconnectCalled {
		t.Errorf("Disconnect request not sent to server")
	}

	c.mu.Lock()
	if c.running {
		t.Errorf("Client should not be running after disconnect")
	}
	c.mu.Unlock()
}

func TestConcurrentSubscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		resp := []message.BayeuxMessage{{
			Channel:      "/meta/subscribe",
			Successful:   boolPtr(true),
			Subscription: reqMsgs[0].Subscription,
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 10; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := c.Subscribe("/foo", func(msg *message.BayeuxMessage) {})
			if err != nil {
				t.Errorf("Concurrent subscribe %d failed: %v", i, err)
			}
		}(i)
	}

	wg.Wait()

	c.handlersMu.RLock()
	count := len(c.handlers["/foo"])
	c.handlersMu.RUnlock()

	if count != 10 {
		t.Errorf("Expected 10 handlers, got %d", count)
	}
}

func TestConcurrentUnsubscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqMsgs []message.BayeuxMessage
		_ = json.NewDecoder(r.Body).Decode(&reqMsgs)

		resp := []message.BayeuxMessage{{
			Channel:      "/meta/subscribe",
			Successful:   boolPtr(true),
			Subscription: reqMsgs[0].Subscription,
		}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.clientID = "test-client-id"

	var unsubscribes []func()
	for i := 0; i < 10; i++ {
		unsub, err := c.Subscribe("/foo", func(msg *message.BayeuxMessage) {})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		unsubscribes = append(unsubscribes, unsub)
	}

	var wg sync.WaitGroup
	wg.Add(len(unsubscribes))

	for i, unsub := range unsubscribes {
		go func(i int, unsub func()) {
			defer wg.Done()
			unsub()
		}(i, unsub)
	}

	wg.Wait()

	c.handlersMu.RLock()
	count := len(c.handlers["/foo"])
	c.handlersMu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 handlers after unsubscribe, got %d", count)
	}
}

func TestMessageDispatch(t *testing.T) {
	messageCount := 0
	var mu sync.Mutex

	c := NewClient("http://example.com/bayeux")
	c.clientID = "test-client-id"

	c.Subscribe("/foo", func(msg *message.BayeuxMessage) {
		mu.Lock()
		messageCount++
		mu.Unlock()
	})

	c.Subscribe("/foo", func(msg *message.BayeuxMessage) {
		mu.Lock()
		messageCount++
		mu.Unlock()
	})

	msgs := []message.BayeuxMessage{
		{Channel: "/foo", Data: map[string]interface{}{"msg": "hello"}},
		{Channel: "/foo", Data: map[string]interface{}{"msg": "world"}},
		{Channel: "/bar", Data: map[string]interface{}{"msg": "ignored"}},
	}

	for _, msg := range msgs {
		c.handlersMu.RLock()
		handlers := c.handlers[msg.Channel]
		c.handlersMu.RUnlock()

		for _, entry := range handlers {
			entry.handler(&msg)
		}
	}

	if messageCount != 4 {
		t.Errorf("Expected 4 handler invocations, got %d", messageCount)
	}
}
