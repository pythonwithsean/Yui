package yui

import (
	"errors"
	"io"
	"net"
	"time"
)

var (
	errHeaderTooLarge = errors.New("header too large")
	errBodyTooLarge   = errors.New("body too large")
	errInvalidBodyLen = errors.New("invalid content-length")
	errIncompleteBody = errors.New("incomplete body")
)

type byteStreamReader struct {
	conn       net.Conn
	data       []byte
	readBuffer []byte
	config     ServerConfig
}

func newByteStreamReader(conn net.Conn, config ServerConfig) *byteStreamReader {
	return &byteStreamReader{
		conn:       conn,
		data:       make([]byte, 0, config.MaxHeaderSize),
		readBuffer: make([]byte, config.MaxHeaderSize),
		config:     config,
	}
}

func (r *byteStreamReader) readMore(phaseDeadline time.Time, idleDeadline *time.Time) (int, error) {
	deadline := phaseDeadline
	if r.config.IdleTimeout > 0 {
		// If the idle deadline is not set or has passed, set it to now + idle timeout
		if idleDeadline.IsZero() || time.Now().After(*idleDeadline) {
			*idleDeadline = time.Now().Add(r.config.IdleTimeout)
		}
		// If the idle deadline is sooner than the phase deadline, use the idle deadline
		if deadline.IsZero() || idleDeadline.Before(deadline) {
			deadline = *idleDeadline
		}
	}
	// Set the read deadline on the connection
	if err := r.conn.SetReadDeadline(deadline); err != nil {
		return 0, err
	}

	n, err := r.conn.Read(r.readBuffer)
	if n > 0 {
		r.data = append(r.data, r.readBuffer[:n]...)
		if r.config.IdleTimeout > 0 {
			*idleDeadline = time.Now().Add(r.config.IdleTimeout)
		}
	}
	return n, err
}

func (r *byteStreamReader) readHeader() ([]byte, []byte, error) {
	phaseDeadline := deadlineAfter(r.config.HeaderTimeout)
	var idleDeadline time.Time

	for {
		if index := indexHeaderEnd(r.data); index >= 0 {
			headerEnd := index + len(CRLF+CRLF)
			if headerEnd > r.config.MaxHeaderSize {
				return nil, nil, errHeaderTooLarge
			}
			header := r.data[:headerEnd]
			r.data = r.data[headerEnd:]
			return header, r.data, nil
		}
		if len(r.data) >= r.config.MaxHeaderSize {
			return nil, nil, errHeaderTooLarge
		}

		n, err := r.readMore(phaseDeadline, &idleDeadline)
		if n > 0 {
			if index := indexHeaderEnd(r.data); index >= 0 {
				headerEnd := index + len(CRLF+CRLF)
				if headerEnd > r.config.MaxHeaderSize {
					return nil, nil, errHeaderTooLarge
				}
				header := r.data[:headerEnd]
				r.data = r.data[headerEnd:]
				return header, r.data, nil
			}
			if len(r.data) >= r.config.MaxHeaderSize {
				return nil, nil, errHeaderTooLarge
			}
		}
		if err != nil {
			return nil, nil, err
		}
	}
}

func (r *byteStreamReader) readBody(bodyLength int) ([]byte, error) {
	if bodyLength < 0 {
		return nil, errInvalidBodyLen
	}
	if bodyLength > r.config.MaxBodySize {
		return nil, errBodyTooLarge
	}

	phaseDeadline := deadlineAfter(r.config.BodyTimeout)
	var idleDeadline time.Time
	for len(r.data) < bodyLength {
		n, err := r.readMore(phaseDeadline, &idleDeadline)
		if len(r.data) >= bodyLength {
			body := r.data[:bodyLength]
			r.data = r.data[bodyLength:]
			return body, nil
		}
		if n > 0 && err == nil {
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errIncompleteBody
			}
			return nil, err
		}
	}

	body := r.data[:bodyLength]
	r.data = r.data[bodyLength:]
	return body, nil
}

func indexHeaderEnd(data []byte) int {
	for i := 0; i+len(CRLF+CRLF) <= len(data); i++ {
		if data[i] == '\r' && data[i+1] == '\n' && data[i+2] == '\r' && data[i+3] == '\n' {
			return i
		}
	}
	return -1
}

func deadlineAfter(timeout time.Duration) time.Time {
	if timeout <= 0 {
		return time.Time{}
	}
	return time.Now().Add(timeout)
}
