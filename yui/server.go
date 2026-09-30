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

const max_header_size = 8192               // 8KB
const max_chunk_size = 1                   // 1 Byte
const max_conn_duration = 30 * time.Second // 30 seconds
const CRLF = "\r\n"                        // Carriage Return + Line Feed
const max_body_size = 1024 * 1024          // 1MB
const max_payload_size = 1024 * 1024 * 10  // 10MB

type HTTPServer struct {
	addr     string
	port     string
	listener net.Listener
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
	StatusOk                  Statuscode = 200
	StatusBadRequest          Statuscode = 400
	StatusNotFound            Statuscode = 404
	StatusRequestTimeout      Statuscode = 408
	StatusLengthRequired      Statuscode = 411
	StatusPayloadTooLarge     Statuscode = 413
	StatusTooManyRequests     Statuscode = 429
	StatusInternalServerError Statuscode = 500
	StatusNotImplemented      Statuscode = 501
	StatusBadGateway          Statuscode = 502
	StatusServiceUnavailable  Statuscode = 503
	StatusGatewayTimeout      Statuscode = 504
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

func NewServer() *HTTPServer {
	return &HTTPServer{}
}

func newListener(addr string) net.Listener {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(fmt.Sprintf("Error starting server on %s: %v", addr, err))
	}
	return listener
}

// newTLSListener returns a listener that speaks TLS. The listener it hands back
// is still a net.Listener, and the connections it accepts are still net.Conn, so
// nothing downstream of Accept has to know encryption is happening.
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
	case StatusOk, StatusBadRequest, StatusNotFound, StatusRequestTimeout, StatusLengthRequired, StatusPayloadTooLarge, StatusTooManyRequests, StatusInternalServerError, StatusNotImplemented, StatusBadGateway, StatusServiceUnavailable, StatusGatewayTimeout:
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
			fmt.Printf("Error with connection from %s\n", conn.RemoteAddr().String())
			continue
		}
		fmt.Printf("🔥 Yui handling Connection from %s\n", conn.RemoteAddr().String())
		go handleConn(conn)
	}
}

func readHeader(conn net.Conn, data *[]byte, buff []byte) (int, error) {

	for {
		// Set a deadline for the connection to avoid hanging connections
		n, err := conn.Read(buff)
		if n > 0 {
			*(data) = append(*(data), buff[:n]...)
		}
		if err != nil {
			// Check if the error is a timeout error
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				writeErrorResponse(conn, "408")
				fmt.Printf("Timeout reading from connection: %s\n", err)
				return -1, err
			}

			// Client closed the connection; process whatever data we have
			if errors.Is(err, io.EOF) {
				return -1, err
			}

			// For other errors, send a 400 Bad Request response
			writeErrorResponse(conn, "400")
			fmt.Printf("Error reading from connection: %s\n", err)
			return -1, err
		}

		// Check if the accumulated data exceeds the maximum header size
		if len(*data) > max_header_size {
			writeErrorResponse(conn, "413")
			fmt.Printf("Header too large from %s\n", conn.RemoteAddr().String())
			return -1, errors.New("header too large")
		}

		// Check if we have received the end of the header section
		if idx := strings.Index(string(*data), CRLF+CRLF); idx != -1 {
			return idx + len(CRLF+CRLF), nil
		}
	}
}

func readBody(conn net.Conn, data *[]byte, buff []byte, headerIdx int, contentLength int) (int, error) {

	// A negative Content-Length is malformed; guard before it reaches a slice bound
	if contentLength < 0 {
		writeErrorResponse(conn, "400")
		fmt.Printf("Negative Content-Length from %s\n", conn.RemoteAddr().String())
		return -1, errors.New("negative content-length")
	}

	// Check if the declared body exceeds the maximum body size
	if contentLength > max_body_size {
		writeErrorResponse(conn, "413")
		fmt.Printf("Body too large from %s\n", conn.RemoteAddr().String())
		return -1, errors.New("body too large")
	}

	// readHeader may have already buffered part (or all) of the body
	for len(*data)-headerIdx < contentLength {
		n, err := conn.Read(buff)
		if n > 0 {
			*(data) = append(*(data), buff[:n]...)
		}
		if err != nil {
			// Check if the error is a timeout error
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				writeErrorResponse(conn, "408")
				fmt.Printf("Timeout reading from connection: %s\n", err)
				return -1, err
			}

			// Client closed before sending the body it promised
			if errors.Is(err, io.EOF) {
				writeErrorResponse(conn, "400")
				fmt.Printf("Incomplete body from %s: got %d of %d bytes\n", conn.RemoteAddr().String(), len(*data)-headerIdx, contentLength)
				return -1, errors.New("incomplete body")
			}

			// For other errors, send a 400 Bad Request response
			writeErrorResponse(conn, "400")
			fmt.Printf("Error reading from connection: %s\n", err)
			return -1, err
		}
	}

	return headerIdx + contentLength, nil
}

func handleConn(conn net.Conn) {
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

	data := make([]byte, 0, max_payload_size) // Initialize a slice to hold the raw data
	buff := make([]byte, max_chunk_size)      // 1 byte buffer

	// if json header has connection: keep-alive, we should keep the connection open for a certain duration
	conn.SetDeadline(time.Now().Add(max_conn_duration)) // Set a deadline for the connection

	// Read the raw connection data and parse the header and body
	headerIdx, err := readHeader(conn, &data, buff)
	if err != nil {
		fmt.Printf("Error reading header from connection: %s\n", err)
		return
	}
	if headerIdx == -1 {
		writeErrorResponse(conn, "400")
		fmt.Printf("No header found in connection: %s\n", conn.RemoteAddr().String())
		return
	}

	headerBuff := strings.TrimRight(string(data[:headerIdx]), CRLF)

	// Create the Request Object
	req := &Request{Headers: make(map[string]string), conn: &conn}
	res := &Response{headers: make(map[string]string), conn: &conn}

	err = MakeHeader(req, strings.Split(headerBuff, CRLF))
	if err != nil {
		fmt.Printf("Error making header: %s\n", err)
		return
	}
	var bodyBuff []byte

	contentLengthString, ok := req.Headers["content-length"]

	if ok {
		contentLength, err := strconv.Atoi(contentLengthString)
		if err != nil {
			writeErrorResponse(conn, "400")
			fmt.Printf("Invalid Content-Length %q from %s\n", contentLengthString, conn.RemoteAddr().String())
			return
		}

		// Read the body and parse it onto the Request
		bodyIdx, err := readBody(conn, &data, buff, headerIdx, contentLength)
		if err != nil {
			fmt.Printf("Error reading body from connection: %s\n", err)
			return
		}

		bodyBuff = data[headerIdx:bodyIdx]
		ParseBody(req, string(bodyBuff))
	}

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
