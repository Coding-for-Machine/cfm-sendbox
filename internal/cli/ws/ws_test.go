package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func echoServer(t *testing.T, handler func(c *Conn)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := Accept(w, r)
		if err != nil {
			return
		}
		handler(c)
	}))
}

func TestEchoAndClose(t *testing.T) {
	srv := echoServer(t, func(c *Conn) {
		for {
			op, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			if string(data) == "bye" {
				c.CloseWith(4999, "Job Completed")
				return
			}
			_ = c.WriteMessage(op, data)
		}
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx, srv.URL+"/api/v2/connect?x=1") // http:// is accepted
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// small, 16-bit and 64-bit length frames
	for _, size := range []int{5, 200, 70000} {
		msg := strings.Repeat("a", size)
		if err := c.WriteText(msg); err != nil {
			t.Fatal(err)
		}
		op, got, err := c.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if op != OpText || string(got) != msg {
			t.Fatalf("size %d: echo mismatch (op %d, len %d)", size, op, len(got))
		}
	}

	_ = c.WriteText("bye")
	_, _, err = c.ReadMessage()
	ce, ok := err.(*CloseError)
	if !ok || ce.Code != 4999 || ce.Reason != "Job Completed" {
		t.Fatalf("want close 4999, got %#v", err)
	}
}

func TestPingAndFragments(t *testing.T) {
	srv := echoServer(t, func(c *Conn) {
		// ping, then a fragmented message: "hel" + "lo"
		c.wmu.Lock()
		_ = c.writeFrame(OpPing, []byte("p"))
		// manual fragments (server -> client, unmasked)
		c.nc.Write([]byte{byte(OpText), 3, 'h', 'e', 'l'})           // FIN=0
		c.nc.Write([]byte{0x80 | byte(OpContinuation), 2, 'l', 'o'}) // FIN=1
		c.wmu.Unlock()
		// wait for the pong
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	})
	defer srv.Close()

	c, err := Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, got, err := c.ReadMessage()
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDialErrors(t *testing.T) {
	plain := httptest.NewServer(http.NotFoundHandler())
	defer plain.Close()
	if _, err := Dial(context.Background(), plain.URL); err == nil {
		t.Fatal("expected handshake error against a plain HTTP server")
	}
	if _, err := Dial(context.Background(), "ftp://x"); err == nil {
		t.Fatal("expected scheme error")
	}
}

func TestConnectionLost(t *testing.T) {
	srv := echoServer(t, func(c *Conn) { c.nc.Close() })
	defer srv.Close()
	c, err := Dial(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = c.ReadMessage()
	if ce, ok := err.(*CloseError); !ok || ce.Code != 1006 {
		t.Fatalf("want 1006 close error, got %#v", err)
	}
}
