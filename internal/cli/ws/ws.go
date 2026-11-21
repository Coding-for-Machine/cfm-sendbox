// Package ws is a minimal RFC 6455 WebSocket implementation (standard library
// only). It has a client (Dial) for the CLI and a tiny server side (Accept)
// that is used by the tests.
package ws

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Frame opcodes.
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

// MaxMessageSize bounds a single (reassembled) message.
const MaxMessageSize = 32 << 20

// CloseError is returned by ReadMessage once the peer closed the connection.
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("websocket closed: %d %s", e.Code, e.Reason)
}

// Conn is a WebSocket connection.
type Conn struct {
	nc     net.Conn
	br     *bufio.Reader
	client bool

	wmu       sync.Mutex
	closeSent bool
}

func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + guid))
	return base64.StdEncoding.EncodeToString(h[:])
}

// Dial opens a WebSocket. rawURL may use ws, wss, http or https.
func Dial(ctx context.Context, rawURL string) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	secure := false
	switch u.Scheme {
	case "ws", "http":
	case "wss", "https":
		secure = true
	default:
		return nil, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	host := u.Host
	if u.Port() == "" {
		if secure {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}

	var d net.Dialer
	nc, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	if secure {
		tc := tls.Client(nc, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tc.HandshakeContext(ctx); err != nil {
			nc.Close()
			return nil, err
		}
		nc = tc
	}

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		nc.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(raw[:])

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, u.Host, key)
	if _, err := io.WriteString(nc, req); err != nil {
		nc.Close()
		return nil, err
	}

	br := bufio.NewReader(nc)
	httpReq, _ := http.NewRequest(http.MethodGet, rawURL, nil)
	resp, err := http.ReadResponse(br, httpReq)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("websocket handshake: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		nc.Close()
		return nil, fmt.Errorf("websocket handshake failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		resp.Header.Get("Sec-WebSocket-Accept") != acceptKey(key) {
		nc.Close()
		return nil, errors.New("websocket handshake failed: bad upgrade response")
	}
	return &Conn{nc: nc, br: br, client: true}, nil
}

// Accept upgrades an HTTP request to a WebSocket (server side).
func Accept(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "not a websocket handshake", http.StatusBadRequest)
		return nil, errors.New("not a websocket handshake")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return nil, errors.New("hijacking unsupported")
	}
	nc, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	if _, err := io.WriteString(nc, resp); err != nil {
		nc.Close()
		return nil, err
	}
	return &Conn{nc: nc, br: brw.Reader, client: false}, nil
}

// WriteMessage sends one unfragmented frame.
func (c *Conn) WriteMessage(op int, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.writeFrame(op, payload)
}

// WriteText sends a text message.
func (c *Conn) WriteText(s string) error { return c.WriteMessage(OpText, []byte(s)) }

func (c *Conn) writeFrame(op int, payload []byte) error {
	hdr := make([]byte, 0, 14)
	hdr = append(hdr, 0x80|byte(op))
	maskBit := byte(0)
	if c.client {
		maskBit = 0x80
	}
	switch n := len(payload); {
	case n < 126:
		hdr = append(hdr, maskBit|byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, maskBit|126, 0, 0)
		binary.BigEndian.PutUint16(hdr[len(hdr)-2:], uint16(n))
	default:
		hdr = append(hdr, maskBit|127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(hdr[len(hdr)-8:], uint64(n))
	}

	body := payload
	if c.client {
		var mask [4]byte
		if _, err := rand.Read(mask[:]); err != nil {
			return err
		}
		hdr = append(hdr, mask[:]...)
		body = make([]byte, len(payload))
		for i, b := range payload {
			body[i] = b ^ mask[i%4]
		}
	}
	_, err := c.nc.Write(append(hdr, body...))
	return err
}

func (c *Conn) readFrame() (fin bool, op int, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(c.br, h[:]); err != nil {
		return
	}
	fin = h[0]&0x80 != 0
	if h[0]&0x70 != 0 {
		err = errors.New("websocket: reserved bits set")
		return
	}
	op = int(h[0] & 0x0F)
	masked := h[1]&0x80 != 0
	if c.client && masked {
		err = errors.New("websocket: server frame must not be masked")
		return
	}
	if !c.client && !masked {
		err = errors.New("websocket: client frame must be masked")
		return
	}

	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(c.br, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(c.br, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > MaxMessageSize {
		err = errors.New("websocket: message too large")
		return
	}
	if op >= OpClose && (n > 125 || !fin) {
		err = errors.New("websocket: invalid control frame")
		return
	}

	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

// ReadMessage returns the next text or binary message. Pings are answered and
// pongs are skipped. When the peer closes, a *CloseError is returned.
func (c *Conn) ReadMessage() (op int, data []byte, err error) {
	var (
		buf   []byte
		first = -1
	)
	for {
		fin, fop, payload, err := c.readFrame()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return 0, nil, &CloseError{Code: 1006, Reason: "connection lost"}
			}
			return 0, nil, err
		}
		switch fop {
		case OpPing:
			c.wmu.Lock()
			_ = c.writeFrame(OpPong, payload)
			c.wmu.Unlock()
		case OpPong:
		case OpClose:
			ce := &CloseError{Code: 1005}
			if len(payload) >= 2 {
				ce.Code = int(binary.BigEndian.Uint16(payload))
				ce.Reason = string(payload[2:])
			}
			c.sendClose(ce.Code, "")
			return 0, nil, ce
		case OpText, OpBinary:
			if first != -1 {
				return 0, nil, errors.New("websocket: new message started before previous one finished")
			}
			first, buf = fop, payload
			if fin {
				return first, buf, nil
			}
		case OpContinuation:
			if first == -1 {
				return 0, nil, errors.New("websocket: unexpected continuation frame")
			}
			buf = append(buf, payload...)
			if len(buf) > MaxMessageSize {
				return 0, nil, errors.New("websocket: message too large")
			}
			if fin {
				return first, buf, nil
			}
		default:
			return 0, nil, fmt.Errorf("websocket: unknown opcode %d", fop)
		}
	}
}

func (c *Conn) sendClose(code int, reason string) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closeSent {
		return
	}
	c.closeSent = true
	p := make([]byte, 2, 2+len(reason))
	binary.BigEndian.PutUint16(p, uint16(code))
	p = append(p, reason...)
	_ = c.writeFrame(OpClose, p)
}

// CloseWith sends a close frame with the given code and reason, then closes
// the underlying connection.
func (c *Conn) CloseWith(code int, reason string) error {
	c.sendClose(code, reason)
	return c.nc.Close()
}

// Close sends a normal-closure frame and closes the connection.
func (c *Conn) Close() error { return c.CloseWith(1000, "") }
