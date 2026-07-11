package yui

import (
	"log"
	"strings"
)

var validMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"DELETE":  true,
	"PATCH":   true,
	"HEAD":    true,
	"OPTIONS": true,
}

var validVersions = map[string]bool{
	"HTTP/1.0": true,
	"HTTP/1.1": true,
	"HTTP/2":   true,
	"HTTP/2.0": true,
}

// IsValidMethod checks if the provided HTTP method is valid.
// HTTP is case-sensitive, so we convert the method to uppercase before checking.
func IsValidMethod(method string) bool {
	return validMethods[strings.ToUpper(method)]
}

// IsValidPath checks if the provided path is valid.
func IsValidPath(path string) bool {
	return strings.HasPrefix(path, "/")
}

// IsValidVersion checks if the provided HTTP version is valid.
// HTTP is case-sensitive, so we convert the version to uppercase before checking.
func IsValidVersion(version string) bool {
	return validVersions[strings.ToUpper(version)]
}

// Empty checks if a string is empty or contains only whitespace.
func Empty(s string) bool {
	return strings.TrimSpace(s) == ""
}

// IsValidHeaderKey checks if the provided header key is valid.
func IsValidHeaderKey(key string) bool {
	return !Empty(key) && !strings.ContainsAny(key, ": ") && (len(strings.Split(strings.TrimSpace(key), " ")) == 1) && IsValidHeaderValue(key)
}

// Ensures no CRLF attack in header is passed
func IsValidHeaderValue(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]

		if c == 0x20 || c == 0x09 {
			continue
		}
		if c < 0x21 || c > 0x7E {
			// control char, DEL, non-ASCII byte
			return false
		}
	}
	return true
}

func ParseHeaderField(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	if !IsValidHeaderKey(key) {
		return "", "", false
	}
	value := strings.TrimSpace(parts[1])
	if Empty(key) || Empty(value) {
		return "", "", false
	}
	return key, value, true
}

func ParseHeader(req *Request, header []string) {
	if len(header) == 0 {
		return
	}

	parts := strings.Split(strings.TrimSpace(header[0]), " ")
	if len(parts) != 3 {
		log.Printf("Invalid request line: expected 3 parts, got %d: %q", len(parts), header[0])
		return
	}
	if !IsValidMethod(parts[0]) {
		log.Printf("Invalid HTTP method: %q", parts[0])
		return
	}
	if !IsValidPath(parts[1]) {
		log.Printf("Invalid path: %q (must start with /)", parts[1])
		return
	}
	if !IsValidVersion(parts[2]) {
		log.Printf("Invalid HTTP version: %q", parts[2])
		return
	}
	if !IsValidHeaderValue(parts[0]) || !IsValidHeaderValue(parts[1]) || !IsValidHeaderValue(parts[2]) {
		log.Printf("Invalid characters in request line: %q", header[0])
		return
	}
	req.Method = strings.ToLower(parts[0])
	req.Path = strings.ToLower(parts[1])
	req.Version = strings.ToLower(parts[2])

	//TODO: duplicate req.Headers key overwite each other
	//TODO-SOL: use a map[string][]string to store multiple values for the same header key, or use a custom struct to hold the headers and their values. This way, you can preserve all header values without overwriting them.
	for i := 1; i < len(header); i++ {
		key, value, ok := ParseHeaderField(header[i])
		if !ok {
			log.Printf("Invalid header: %q", header[i])
			continue
		}
		if !IsValidHeaderValue(value) {
			log.Printf("Invalid characters in header: %q", header[i])
			continue
		}
		// NOTE: im making everything lower case so i dont have to worry about casing ever in processing
		req.Headers[strings.ToLower(key)] = strings.ToLower(value)
		if strings.ToUpper(key) == "HOST" {
			req.Host = strings.ToLower(value)
		}
	}
}

func ParseBody(req *Request, body string) {
	if !Empty(body) {
		req.Body = body
	}
}
