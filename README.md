# Yui

An HTTP/1.1 server written from scratch in Go, on top of raw TCP sockets. No `net/http`, no frameworks. I built it to understand what actually happens between `accept()` and a response showing up in the browser.

**Status: finished.** This was a learning project and it has done its job. The code stays here as a record of what I learned; it is not meant for production use.

## What it does

- Accepts TCP connections and handles each one in its own goroutine
- Reads the request byte by byte until `\r\n\r\n`, then reads exactly `Content-Length` bytes of body
- Parses and validates the request line (method, path, version) and header fields
- Rejects control characters and non-ASCII bytes in headers, which blocks CRLF/header injection
- Enforces limits: 8KB of headers, 1MB of body, a 30 second connection deadline
- Returns proper error responses: `400`, `404`, `408`, `413`
- Routes `GET`, `POST`, `PUT` and `DELETE` by method and path to user-defined handlers
- Builds responses with a chainable API (`Status`, `SetHeader`, `Send`) and sets `Content-Length` for you
- Serves HTTPS through `ListenAndServeTLS` (TLS 1.2+, ALPN pinned to `http/1.1`)

## Usage

```go
package main

import "github.com/pythonwithsean/Yui/yui"

func main() {
	s := yui.NewServer()

	s.Get("/", func(req *yui.Request, res *yui.Response) {
		res.SetHeader("Content-Type", "text/html")
		res.Status(yui.StatusOk).Send("<h1>Hello from Yui</h1>")
	})

	s.Post("/echo", func(req *yui.Request, res *yui.Response) {
		res.Status(yui.StatusOk).Send(req.Body)
	})

	s.ListenAndServe("localhost", ":8000")
	// or: s.ListenAndServeTLS("localhost", ":8443", "cert.pem", "key.pem")
}
```

```bash
make run     # hot reload with air
go run .     # or run it directly
make curl    # curl -v localhost:8000
```

## Layout

```
main.go            Example app
yui/server.go      Listener, connection handling, reading, routing, responses, TLS
yui/parser.go      Request line and header parsing and validation
http-protocol.md   My implementer's summary of RFC 9110 / 9112
notes.md           Go notes and the header-injection write-up
```

## What I learned

### Networking and the OS
- A listening socket and a connection socket are different file descriptors. One accepts, the other reads and writes.
- TCP is a byte stream, not a message stream. A single `Read` can return half a request or a request and a half, so the server has to buffer and look for boundaries itself.
- Deadlines matter. Without one, a client that opens a connection and sends nothing holds a goroutine forever.
- Ports: well-known (0–1023), registered (1024–49151), ephemeral (49152–65535).

### HTTP
- A message is a start line, header lines, a blank line, and an optional body. Every line ends in `\r\n`.
- The body has no terminator. `Content-Length` is the only thing telling you where it ends, so a missing, negative or non-numeric value has to be rejected.
- Headers are untrusted input. A raw `\r\n` inside a header value lets an attacker inject new headers, so validation happens at the byte level (`0x21`–`0x7E` plus space and tab).
- Reading the RFCs means reading ABNF. `notes.md` has the cheat sheet.

### TLS
- TLS wraps the connection but keeps the same `net.Listener` / `net.Conn` interfaces, so the HTTP code above it did not change at all.
- The handshake is lazy in Go. Forcing it right after `Accept` stops the server from writing plaintext errors into a connection the client expects to be encrypted.
- ALPN decides the protocol. If you don't pin it to `http/1.1`, a browser may negotiate HTTP/2 and send binary frames a text parser can't read.

### Go
- `make` versus the zero value: writing to a nil map panics.
- Strings index as bytes, not runes, which is why a byte-range check rejects all non-ASCII input without decoding UTF-8.
- `defer` only runs if execution reaches the `defer` statement.
- Interfaces are satisfied implicitly, which is what made the TLS swap free.

## Known limitations

These are the parts I chose not to build. Each one would be the next step if I came back to this.

- **No keep-alive.** Every connection serves exactly one request and then closes.
- **No chunked transfer encoding.** Request bodies must use `Content-Length`.
- **Reads one byte per syscall.** Simple to reason about, slow in practice.
- **Lowercases the path and every header value**, so `/Users` and `/users` are the same route and header values lose their case.
- **Duplicate headers overwrite each other** instead of being kept as a list.
- **Exact-match routing only.** No path parameters, query-string parsing or middleware.
- **Routes live in a package-level map**, so all servers in a process share them.
- **Accepts `HTTP/2` in the request line** but only speaks HTTP/1.1.
- **`tests/parser_test.go` is out of date.** It still imports the old `httpserver/server` package and does not compile.

## Roadmap

1. ✅ Set up listener socket and accept connections
2. ✅ Read raw bytes from the connection
3. ✅ Parse the HTTP request (method, path, headers, body)
4. ✅ Build a response writer
5. ✅ Implement a router
6. ✅ Handle incomplete reads, size limits and timeouts
7. ✅ TLS
8. ❌ Keep-alive connections (not planned)
