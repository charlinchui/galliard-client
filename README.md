# Galliard Client

**Galliard Client** is a minimal, idiomatic Go client for the [Bayeux protocol](https://docs.cometd.org/current/reference/#_bayeux).  
It lets you connect to any Bayeux-compliant server (including [Galliard Server](https://github.com/charlinchui/galliard)), subscribe to channels, publish messages, and receive real-time events in your Go code.

---

## Features

- **Bayeux protocol**: handshake, subscribe, publish, connect (long-polling), disconnect
- **Channel-based pub/sub**: subscribe to any channel, register Go callbacks
- **Thread-safe**: safe for concurrent use
- **Unsubscribe support**: easily remove handlers
- **Simple, clean API**: just what you need to build real-time Go apps

---

## Implementation Checklist

- [x] Handshake with server
- [x] Subscribe to channels (with Go callback)
- [x] Publish messages to channels
- [x] Long-polling connect loop
- [x] Graceful disconnect
- [x] Unsubscribe handlers
- [ ] Honor server advice (planned)
- [ ] WebSocket transport (planned)
- [ ] More advanced error handling (planned)

---

## Getting Started

### 1. Install

``` bash
go get github.com/charlinchui/galliard-client
```

### 2. Usage Example

``` go
import (
    "github.com/charlinchui/galliard-client/client"
    "github.com/charlinchui/galliard/message"
    "log"
    "time"
)

func main() {
    c := client.NewClient("http://localhost:8000/bayeux")
    if err := c.Handshake(); err != nil {
        log.Fatal("Handshake failed:", err)
    }

    unsub, err := c.Subscribe("/foo", func(msg *message.BayeuxMessage) {
        log.Printf("Received on /foo: %+v", msg)
    })
    if err != nil {
        log.Fatal("Subscribe failed:", err)
    }
    defer unsub()

    if err := c.Connect(); err != nil {
        log.Fatal("Connect failed:", err)
    }

    // Publish a message
    time.Sleep(1 * time.Second)
    if err := c.Publish("/foo", map[string]interface{}{"msg": "hello from client"}); err != nil {
        log.Fatal("Publish failed:", err)
    }

    // Let the client run for a bit to receive messages
    time.Sleep(3 * time.Second)
    c.Disconnect()
}
```

---

## API

- `type Client`  
  The Bayeux client.
- `func NewClient(serverURL string) *Client`  
  Create a new client for the given server URL.
- `func (c *Client) Handshake() error`  
  Perform the Bayeux handshake and store the client ID.
- `func (c *Client) Subscribe(channel string, handler func(*message.BayeuxMessage)) (func(), error)`  
  Subscribe to a channel and register a callback. Returns an unsubscribe function.
- `func (c *Client) Publish(channel string, data map[string]interface{}) error`  
  Publish a message to a channel.
- `func (c *Client) Connect() error`  
  Start the long-polling loop to receive messages.
- `func (c *Client) Disconnect() error`  
  Gracefully disconnect from the server.

---

## Running Tests

``` bash
go test ./...
```

---

## Contributing

Contributions are welcome!  
If you have ideas, bug reports, or want to help with features, please open an issue or pull request.

---

## License

MIT License

---

## References

- [Bayeux Protocol Specification](https://docs.cometd.org/current/reference/#_bayeux)
- [CometD Project](https://cometd.org/)
- [Faye Project](https://faye.jcoglan.com/)

---

**Galliard Client**: Real-time, reliable, and readable Bayeux for Go.
