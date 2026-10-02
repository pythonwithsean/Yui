package yui

import (
	"net"
	"testing"
	"time"
)

func TestByteStreamReaderKeepsBodyBytesReadWithHeader(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	config := DefaultServerConfig()
	config.HeaderTimeout = time.Second
	config.BodyTimeout = time.Second
	config.IdleTimeout = time.Second
	reader := newByteStreamReader(serverConn, config)

	go func() {
		_, _ = clientConn.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello"))
	}()

	header, _, err := reader.readHeader()
	if err != nil {
		t.Fatalf("readHeader() returned an error: %v", err)
	}
	if string(header) != "GET / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\n" {
		t.Fatalf("unexpected header: %q", header)
	}

	body, err := reader.readBody(5)
	if err != nil {
		t.Fatalf("readBody() returned an error: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestByteStreamReaderRejectsOversizedHeader(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	config := DefaultServerConfig()
	config.MaxHeaderSize = 16
	reader := newByteStreamReader(serverConn, config.withDefaults())

	go func() {
		_, _ = clientConn.Write([]byte("GET / HTTP/1.1\r\n"))
	}()

	_, _, err := reader.readHeader()
	if err != errHeaderTooLarge {
		t.Fatalf("readHeader() error = %v, want %v", err, errHeaderTooLarge)
	}
}
