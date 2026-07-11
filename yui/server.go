package yui

import (
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

type HTTPServer struct {
	addr     string
	port     string
	listener net.Listener
}

type Request struct {
	Method  string
	Path    string
	Version string
	Host    string
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

func (s *Response) buildHeader() string {
	return ""
}

func (s *Response) buildBody() string {
	return ""
}

func (s *Response) statusCodeToString() string {
	switch s.status {
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
	s.headers[key] = value
	return s
}

func (s *Response) Setbody(body string) *Response {
	s.body = body
	return s
}

func (s *Response) Send(msg string) {
	c := *s.conn
	c.Write([]byte(fmt.Sprintf("HTTP/1.1 %s %s\r\nContent-Length: %d\r\nContent-Type: text/html\r\n\r\n%s", s.status, msg, len(s.body), msg)))
}

func (s *HTTPServer) ListenAndServe(addr, port string) {
	s.addr = addr
	s.port = port
	listener := newListener(s.addr + s.port)
	s.listener = listener
	defer s.listener.Close()
	fmt.Printf("🔥 Yui Server Listening on %s\n", s.addr+s.port)
	s.handleConnections()
}

func (s *HTTPServer) Get(path string, handler func(req *Request, res *Response)) {
	verb := "get"
	key := verb + ":" + path
	HandlerMap[key] = handler
}

func (s *HTTPServer) Post(path string, handler func(req *Request, res *Response)) {
	verb := "post"
	key := verb + ":" + path
	HandlerMap[key] = handler

}

func (s *HTTPServer) Put(path string, handler func(req *Request, res *Response)) {
	verb := "put"
	key := verb + ":" + path
	HandlerMap[key] = handler

}

func (s *HTTPServer) Delete(path string, handler func(req *Request, res *Response)) {
	verb := "delete"
	key := verb + ":" + path
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

func handleConn(conn net.Conn) {
	defer conn.Close()
	var data []byte
	chunk := make([]byte, max_chunk_size) // 1 byte buffer
	var headerBlock []byte
	var bodyBlockIdx int
	// if json header has connection: keep-alive, we should keep the connection open for a certain duration
	conn.SetDeadline(time.Now().Add(max_conn_duration)) // Set a deadline for the connection
	for {
		// Set a deadline for the connection to avoid hanging connections
		n, err := conn.Read(chunk)
		if n > 0 {
			data = append(data, chunk[:n]...)
		}
		if err != nil {
			// Check if the error is a timeout error
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				msg := "HTTP/1.1 408 Request Timeout\r\n\r\n"
				conn.Write([]byte(msg))
				fmt.Printf("Timeout reading from connection: %s\n", err)
				return
			}
			if errors.Is(err, io.EOF) {
				// Client closed the connection; process whatever data we have
				break
			}
			// For other errors, send a 400 Bad Request response
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			fmt.Printf("Error reading from connection: %s\n", err)
			return
		}
		// Check if the accumulated data exceeds the maximum header size
		if len(data) > max_header_size {
			conn.Write([]byte("HTTP/1.1 413 Payload Too Large\r\n\r\n"))
			fmt.Printf("Header too large from %s\n", conn.RemoteAddr().String())
			return
		}
		// Check if we have received the end of the header section
		if idx := strings.Index(string(data), CRLF+CRLF); idx != -1 {
			headerBlock = data[:idx]
			bodyBlockIdx = idx + len(CRLF+CRLF)
			break
		}
	}

	if len(headerBlock) == 0 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		fmt.Printf("No header received from %s\n", conn.RemoteAddr().String())
		return
	}

	// Create the Request Object
	req := &Request{Headers: make(map[string]string), conn: &conn}
	res := &Response{conn: &conn}
	ParseHeader(req, strings.Split(string(headerBlock), CRLF))
	if req.Method == "" || req.Path == "" || req.Version == "" || req.Host == "" || len(req.Headers) == 0 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		fmt.Printf("Invalid request from %s\n", conn.RemoteAddr().String())
		return
	}

	_, ok := req.Headers["content-length"]
	if ok {
		cl, err := strconv.Atoi(req.Headers["content-length"])
		if err != nil {
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			fmt.Printf("Error reading from connection: %s\n", err)
			return
		}
		if cl > max_body_size {
			conn.Write([]byte("HTTP/1.1 413 Payload Too Large\r\n\r\n"))
			fmt.Printf("Body too large from %s\n", conn.RemoteAddr().String())
			return
		}
		remaining_body_bytes := cl - (len(data) - bodyBlockIdx)
		if remaining_body_bytes > 0 {
			// Read the remaining body bytes
			bodyChunk := make([]byte, remaining_body_bytes)
			n, err := conn.Read(bodyChunk)
			if err != nil {
				conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
				fmt.Printf("Error reading from connection: %s\n", err)
				return
			}
			data = append(data, bodyChunk[:n]...)
		}
		req.Body = string(data[bodyBlockIdx:])
	}
	key := strings.ToLower(req.Method) + ":" + req.Path
	if handler, ok := HandlerMap[key]; ok {
		handler(req, res)
	} else {
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
		fmt.Printf("Error could not resolve path: %s\n", req.Path)
		return
	}
}
