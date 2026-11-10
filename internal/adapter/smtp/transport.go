package smtp

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"time"
)

// go-smtp always enables CHUNKING. Keep submission on DATA until upstream
// verifies the complete length of a BDAT LAST chunk before accepting the message.
type submissionListener struct {
	net.Listener
}

func (l submissionListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	// The SMTP greeting triggers the TLS handshake in the connection's worker.
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &submissionConn{Conn: conn, reader: bufio.NewReader(conn)}, nil
}

type submissionConn struct {
	net.Conn
	reader  *bufio.Reader
	pending []byte
	data    bool
}

func (c *submissionConn) Read(p []byte) (int, error) {
	if len(c.pending) == 0 {
		// Deliver one line per read. Pipelined commands following DATA must
		// pass the command check after go-smtp has sent the final DATA reply.
		line, err := c.reader.ReadSlice('\n')
		if err != nil {
			_ = c.Close()
			return 0, err
		}
		cmd := bytes.TrimRight(line, "\r\n")
		if !c.data && len(cmd) >= 4 && bytes.EqualFold(cmd[:4], []byte("BDAT")) &&
			(len(cmd) == 4 || cmd[4] == ' ') {
			// Chunk bytes can already be pipelined. End this connection so
			// none of those bytes can be interpreted as a new SMTP command.
			_, _ = c.Conn.Write([]byte("502 5.5.1 BDAT unavailable; use DATA\r\n"))
			_ = c.Close()
			return 0, net.ErrClosed
		}
		c.pending = line
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func (c *submissionConn) Write(p []byte) (int, error) {
	// go-smtp flushes each SMTP reply line separately.
	if bytes.Equal(p, []byte("250-CHUNKING\r\n")) {
		return len(p), nil
	}
	c.data = bytes.HasPrefix(p, []byte("354 "))
	if bytes.Equal(p, []byte("250-SIZE\r\n")) {
		_, err := fmt.Fprintf(c.Conn, "250-SIZE %d\r\n", maxMessageBytes)
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return c.Conn.Write(p)
}
