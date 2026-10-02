package yui

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const CRLF = "\r\n"

const (
	defaultMaxHeaderSize = 8192             // 8kb
	defaultMaxBodySize   = 1024 * 1024      // 1mb
	defaultHeaderTimeout = 10 * time.Second // 10 seconds
	defaultBodyTimeout   = 30 * time.Second // 30 seconds
	defaultIdleTimeout   = 5 * time.Second  // 5 seconds
)

type ServerConfig struct {
	MaxHeaderSize int
	MaxBodySize   int
	HeaderTimeout time.Duration
	BodyTimeout   time.Duration
	IdleTimeout   time.Duration
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		MaxHeaderSize: defaultMaxHeaderSize,
		MaxBodySize:   defaultMaxBodySize,
		HeaderTimeout: defaultHeaderTimeout,
		BodyTimeout:   defaultBodyTimeout,
		IdleTimeout:   defaultIdleTimeout,
	}
}

func (c ServerConfig) withDefaults() ServerConfig {
	defaults := DefaultServerConfig()
	if c.MaxHeaderSize <= 0 {
		c.MaxHeaderSize = defaults.MaxHeaderSize
	}
	if c.MaxHeaderSize < len(CRLF+CRLF) {
		c.MaxHeaderSize = len(CRLF + CRLF)
	}
	if c.MaxBodySize <= 0 {
		c.MaxBodySize = defaults.MaxBodySize
	}
	if c.HeaderTimeout <= 0 {
		c.HeaderTimeout = defaults.HeaderTimeout
	}
	if c.BodyTimeout <= 0 {
		c.BodyTimeout = defaults.BodyTimeout
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaults.IdleTimeout
	}
	return c
}

type HTTPServer struct {
	addr     string
	port     string
	listener net.Listener
	config   ServerConfig
}

type Request struct {
	method  string
	Path    string
	version string
	host    string
	Headers map[string]string
	Body    string
	conn    *net.Conn
}

// TODO: Setup Custom Status Code Type

const (
	StatusOk                    Statuscode = 200
	StatusBadRequest            Statuscode = 400
	StatusNotFound              Statuscode = 404
	StatusRequestTimeout        Statuscode = 408
	StatusLengthRequired        Statuscode = 411
	StatusPayloadTooLarge       Statuscode = 413
	StatusRequestHeaderTooLarge Statuscode = 431
	StatusTooManyRequests       Statuscode = 429
	StatusInternalServerError   Statuscode = 500
	StatusNotImplemented        Statuscode = 501
	StatusBadGateway            Statuscode = 502
	StatusServiceUnavailable    Statuscode = 503
	StatusGatewayTimeout        Statuscode = 504
)

type Statuscode int

type Response struct {
	headers map[string]string
	method  string
	status  string
	body    string
	conn    *net.Conn
}

// Maps verb:path -> function defined by the user
var HandlerMap = make(map[string]func(req *Request, res *Response))

func NewServer(config ...ServerConfig) *HTTPServer {
	serverConfig := DefaultServerConfig()
	if len(config) > 0 {
		serverConfig = config[0].withDefaults()
	}
	return &HTTPServer{config: serverConfig}
}

func newListener(addr string) net.Listener {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(fmt.Sprintf("Error starting server on %s: %v", addr, err))
	}
	return listener
}

func newTLSListener(addr, certFile, keyFile string) net.Listener {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panic(fmt.Sprintf("Error loading TLS certificate: %v", err))
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		// ALPN: tell the client we only speak HTTP/1.1. Without this a browser
		// may negotiate h2 and send HTTP/2 binary frames the parser can't read.
		NextProtos: []string{"http/1.1"},
	}

	listener, err := tls.Listen("tcp", addr, config)
	if err != nil {
		panic(fmt.Sprintf("Error starting TLS server on %s: %v", addr, err))
	}
	return listener
}

func (s *Response) buildHeader() string {
	return buildResponseHeader(s.status, s.headers, s.body)
}

func buildResponseHeader(status string, headers map[string]string, body string) string {
	var header strings.Builder
	fmt.Fprintf(&header, "HTTP/1.1 %s %s\r\nContent-Length: %d\r\n", status, statusCodeToString(status), len(body))
	for key, value := range headers {
		fmt.Fprintf(&header, "%s: %s\r\n", key, value)
	}
	header.WriteString(CRLF)
	return header.String()
}

func writeHTTPResponse(conn net.Conn, status string, headers map[string]string, body string) error {
	response := buildResponseHeader(status, headers, body) + body
	for len(response) > 0 {
		n, err := conn.Write([]byte(response))
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		response = response[n:]
	}
	return nil
}

func writeErrorResponse(conn net.Conn, status string) {
	if err := writeHTTPResponse(conn, status, nil, ""); err != nil {
		fmt.Printf("Error writing HTTP %s response: %s\n", status, err)
	}
}

func (s *Response) buildBody() string {
	return fmt.Sprintf("")
}

func (s *Response) statusCodeToString() string {
	return statusCodeToString(s.status)
}

func statusCodeToString(status string) string {
	switch status {
	case "200":
		return "OK"
	case "400":
		return "Bad Request"
	case "404":
		return "Not Found"
	case "408":
		return "Request Timeout"
	case "411":
		return "Length Required"
	case "413":
		return "Payload Too Large"
	case "431":
		return "Request Header Fields Too Large"
	case "429":
		return "Too Many Requests"
	case "500":
		return "Internal Server Error"
	case "501":
		return "Not Implemented"
	case "502":
		return "Bad Gateway"
	case "503":
		return "Service Unavailable"
	case "504":
		return "Gateway Timeout"
	default:
		return "Unknown"
	}
}

func (s *Response) isValidStatusCode(statusCode Statuscode) bool {
	switch statusCode {
	case StatusOk, StatusBadRequest, StatusNotFound, StatusRequestTimeout, StatusLengthRequired, StatusPayloadTooLarge, StatusRequestHeaderTooLarge, StatusTooManyRequests, StatusInternalServerError, StatusNotImplemented, StatusBadGateway, StatusServiceUnavailable, StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (s *Response) Status(statusCode Statuscode) *Response {
	if !s.isValidStatusCode(statusCode) {
		panic(fmt.Sprintf("Invalid status code: %d", statusCode))
	}
	s.status = strconv.Itoa(int(statusCode))
	return s
}

func (s *Response) SetHeader(key, value string) *Response {
	if s.headers == nil {
		s.headers = make(map[string]string)
	}
	s.headers[key] = value
	return s
}

func (s *Response) Setbody(body string) *Response {
	s.body = body
	return s
}

func (s *Response) Send(msg string) {
	s.body = msg
	if err := writeHTTPResponse(*s.conn, s.status, s.headers, msg); err != nil {
		fmt.Printf("Error writing HTTP response: %s\n", err)
	}
}

func (s *HTTPServer) ListenAndServe(addr, port string) {
	if len(HandlerMap) == 0 {
		fmt.Println("Warning: No handlers registered. The server will respond with 404 Not Found for all requests.")
	}
	s.addr = addr
	s.port = port
	listener := newListener(s.addr + s.port)
	s.listener = listener
	defer s.listener.Close()
	fmt.Printf("🔥 Yui Server Listening on %s\n", s.addr+s.port)
	s.handleConnections()
}

// ListenAndServeTLS is ListenAndServe over an encrypted connection. The HTTP
// parsing below it is identical; only the listener differs.
func (s *HTTPServer) ListenAndServeTLS(addr, port, certFile, keyFile string) {
	if len(HandlerMap) == 0 {
		fmt.Println("Warning: No handlers registered. The server will respond with 404 Not Found for all requests.")
	}
	s.addr = addr
	s.port = port
	s.listener = newTLSListener(s.addr+s.port, certFile, keyFile)
	defer s.listener.Close()
	fmt.Printf("🔒 Yui Server Listening on https://%s\n", s.addr+s.port)
	s.handleConnections()
}

func (s *HTTPServer) Get(path string, handler func(req *Request, res *Response)) {
	if HandlerMap == nil {
		HandlerMap = make(map[string]func(req *Request, res *Response))
	}
	key := "get" + ":" + path
	HandlerMap[key] = handler
}

func (s *HTTPServer) Post(path string, handler func(req *Request, res *Response)) {
	if HandlerMap == nil {
		HandlerMap = make(map[string]func(req *Request, res *Response))
	}
	key := "post" + ":" + path
	HandlerMap[key] = handler

}

func (s *HTTPServer) Put(path string, handler func(req *Request, res *Response)) {
	if HandlerMap == nil {
		HandlerMap = make(map[string]func(req *Request, res *Response))
	}
	key := "put" + ":" + path
	HandlerMap[key] = handler

}

func (s *HTTPServer) Delete(path string, handler func(req *Request, res *Response)) {
	if HandlerMap == nil {
		HandlerMap = make(map[string]func(req *Request, res *Response))
	}
	key := "delete" + ":" + path
	HandlerMap[key] = handler
}

func (s *HTTPServer) handleConnections() {
	if s.listener == nil {
		panic("Listener is not initialized")
	}
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %s\n", err)
			continue
		}
		fmt.Printf("🔥 Yui handling Connection from %s\n", conn.RemoteAddr().String())
		go handleConn(conn, s.config)
	}
}

func handleConn(conn net.Conn, config ServerConfig) {
	defer conn.Close()

	// On a TLS connection the handshake is lazy: Accept returns before it runs,
	// and it would otherwise fail part-way through readHeader. Force it here so a
	// failure closes the connection instead of writing plaintext errors into a
	// tunnel the client cannot decrypt.
	if tlsConn, ok := conn.(*tls.Conn); ok {
		if err := tlsConn.Handshake(); err != nil {
			fmt.Printf("TLS handshake failed from %s: %s\n", conn.RemoteAddr().String(), err)
			return
		}
	}

	reader := newByteStreamReader(conn, config)
	headerBytes, _, err := reader.readHeader()
	if err != nil {
		handleReadError(conn, err, "header")
		return
	}

	// Parse the header into a Request object.
	req, err := ParseHeader(headerBytes)
	if err != nil {
		writeErrorResponse(conn, "400")
		fmt.Printf("Error making header: %s\n", err)
		return
	}
	req.conn = &conn
	res := &Response{headers: make(map[string]string), conn: &conn}

	if _, chunked := req.Headers["transfer-encoding"]; chunked {
		writeErrorResponse(conn, "501")
		fmt.Printf("Unsupported Transfer-Encoding from %s\n", conn.RemoteAddr().String())
		return
	}

	contentLengthString, ok := req.Headers["content-length"]

	if ok {
		contentLength, err := strconv.Atoi(contentLengthString)
		if err != nil {
			writeErrorResponse(conn, "400")
			fmt.Printf("Invalid Content-Length %q from %s\n", contentLengthString, conn.RemoteAddr().String())
			return
		}

		bodyBytes, err := reader.readBody(contentLength)
		if err != nil {
			handleReadError(conn, err, "body")
			fmt.Printf("Error reading body from connection: %s\n", err)
			return
		}
		ParseBody(req, string(bodyBytes))
	}
	_ = conn.SetReadDeadline(time.Time{})

	// Handle the request by looking up the appropriate handler in the HandlerMap
	key := strings.ToLower(req.method) + ":" + req.Path
	if handler, ok := HandlerMap[key]; ok {
		handler(req, res)
	} else {
		writeErrorResponse(conn, "404")
		fmt.Printf("Error could not resolve path: %s\n", req.Path)
		return
	}
}

func handleReadError(conn net.Conn, err error, phase string) {
	switch {
	case errors.Is(err, errHeaderTooLarge):
		writeErrorResponse(conn, "431")
	case errors.Is(err, errBodyTooLarge):
		writeErrorResponse(conn, "413")
	case errors.Is(err, errInvalidBodyLen):
		writeErrorResponse(conn, "400")
	case errors.Is(err, errIncompleteBody):
		writeErrorResponse(conn, "400")
	case isTimeoutError(err):
		writeErrorResponse(conn, "408")
	default:
		writeErrorResponse(conn, "400")
	}
	fmt.Printf("Error reading %s from connection: %s\n", phase, err)
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
