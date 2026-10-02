package tests

import (
	"testing"

	httpServer "github.com/pythonwithsean/Yui/yui"
)

func TestMakeHeader(t *testing.T) {
	if _, err := httpServer.ParseHeader([]byte(
		"GET / HTTP/1.1\r\nHost: localhost:5100\r\nUser-Agent: curl/7.64.1\r\nAccept: */*\r\n\r\n",
	)); err != nil {
		t.Fatalf("ParseHeader() returned an error: %v", err)
	}

}

func TestIsValidMethod(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"GET", true},
		{"POST", true},
		{"PUT", true},
		{"DELETE", true},
		{"PATCH", true},
		{"HEAD", true},
		{"OPTIONS", true},
		{"INVALID", false},
		{"get", true},
		{"", false},
	}
	for _, tt := range tests {
		got := httpServer.IsValidMethod(tt.method)
		if got != tt.want {
			t.Errorf("IsValidMethod(%q) = %v, want %v", tt.method, got, tt.want)
		}
	}
}

func TestIsValidPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/users", true},
		{"/users/1", true},
		{"/api/v1/data", true},
		{"users", false},
		{"", false},
	}
	for _, tt := range tests {
		got := httpServer.IsValidPath(tt.path)
		if got != tt.want {
			t.Errorf("IsValidPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"HTTP/1.0", true},
		{"HTTP/1.1", true},
		{"HTTP/2", true},
		{"HTTP/2.0", true},
		{"HTTP/3", false},
		{"http/1.1", true},
		{"", false},
	}
	for _, tt := range tests {
		got := httpServer.IsValidVersion(tt.version)
		if got != tt.want {
			t.Errorf("IsValidVersion(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestIsValidHeaderKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"Host", true},
		{"Content-Length", true},
		{"X-Custom-Header", true},
		{"", false},
		{"   ", false},
		{"Bad Key", false},    // contains a space
		{"Bad:Key", false},    // contains a colon
		{"Bad\r\nKey", false}, // CRLF injection attempt
		{"Évil", false},       // non-ASCII byte
	}
	for _, tt := range tests {
		got := httpServer.IsValidHeaderKey(tt.key)
		if got != tt.want {
			t.Errorf("IsValidHeaderKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestIsValidHeaderValue(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"localhost:5100", true},
		{"curl/7.64.1", true},
		{"*/*", true},
		{"value with spaces", true},
		{"value\twith\ttabs", true},
		{"", true},                               // empty value has no invalid characters
		{"bad\r\nSet-Cookie: admin=true", false}, // CRLF injection attempt
		{"bad\nvalue", false},                    // bare LF
		{"bad\x00value", false},                  // NUL byte
		{"bad\x7Fvalue", false},                  // DEL char
		{"héllo", false},                         // non-ASCII byte (UTF-8 multi-byte)
	}
	for _, tt := range tests {
		got := httpServer.IsValidHeaderValue(tt.value)
		if got != tt.want {
			t.Errorf("IsValidHeaderValue(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
