package astm

import (
	"log"
	"net"
	"time"

	"lightbaseEMRProxy/internal/config"
)

// TCPConn wraps a net.Conn to satisfy the Port interface
type TCPConn struct {
	conn net.Conn
}

func (t *TCPConn) Read(b []byte) (int, error) {
	n, err := t.conn.Read(b)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, nil
		}
		return n, err
	}
	return n, nil
}

func (t *TCPConn) Write(b []byte) (int, error) {
	return t.conn.Write(b)
}

func (t *TCPConn) SetReadTimeout(d time.Duration) error {
	return t.conn.SetReadDeadline(time.Now().Add(d))
}

// StartTCPListener starts the ASTM TCP listener
func StartTCPListener() {
	addr := config.PCIP + ":" + config.ASTMTCPPort
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("❌ [ASTM-TCP] Could not bind %s: %v\n", addr, err)
		return
	}
	defer ln.Close()
	log.Printf("📡 [ASTM-TCP] Listening on %s — waiting for instrument...\n", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("❌ [ASTM-TCP] Accept error:", err)
			continue
		}
		log.Printf("🔌 [ASTM-TCP] Instrument connected: %s\n", conn.RemoteAddr())
		go func(c net.Conn) {
			defer c.Close()
			HandlePort(&TCPConn{conn: c})
			log.Printf("🔌 [ASTM-TCP] Instrument disconnected: %s\n", c.RemoteAddr())
		}(conn)
	}
}
