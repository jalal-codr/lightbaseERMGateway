package astm

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"

	"go.bug.st/serial"
)

// Port interface for both serial and TCP connections
type Port interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	SetReadTimeout(t time.Duration) error
}

// StartSerialListener starts the ASTM serial port listener
func StartSerialListener() {
	mode := &serial.Mode{
		BaudRate: config.ASTMBaudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	log.Printf("📡 [ASTM] Opening %s at %d baud...\n", config.ASTMComPort, config.ASTMBaudRate)

	for {
		port, err := serial.Open(config.ASTMComPort, mode)
		if err != nil {
			log.Printf("❌ [ASTM] Could not open %s: %v — retrying in 5s\n", config.ASTMComPort, err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("✅ [ASTM] %s open — waiting for ENQ from instrument...\n", config.ASTMComPort)
		HandlePort(port)
		port.Close()
		log.Printf("⚠️  [ASTM] Session ended, reopening %s...\n", config.ASTMComPort)
		time.Sleep(1 * time.Second)
	}
}

// HandlePort handles ASTM communication on a port
func HandlePort(port Port) {
	buf := make([]byte, 1)

	for {
		port.SetReadTimeout(30 * time.Second)
		n, err := port.Read(buf)
		if err != nil {
			log.Printf("⚠️  [ASTM] Port error: %v — closing port\n", err)
			return
		}
		if n == 0 {
			log.Println("[ASTM] Idle 30s — still listening for ENQ...")
			continue
		}

		b := buf[0]
		log.Printf("[ASTM] Received: 0x%02X (%s)\n", b, byteDesc(b))

		if b == config.ENQ {
			log.Println("\n📥 [ASTM] ENQ received — sending ACK")
			if _, err := port.Write([]byte{config.ACK}); err != nil {
				log.Println("❌ [ASTM] Failed to send ACK:", err)
				return
			}
			handleSession(port)
		}
	}
}

func handleSession(port Port) {
	type state int
	const (
		idle state = iota
		inFrame
		tail
	)

	var fullMessage strings.Builder
	var frame bytes.Buffer
	frameCount := 0
	tailCount := 0
	lastEndByte := byte(0)
	cur := idle
	buf := make([]byte, 1)

	readByte := func() (byte, bool) {
		port.SetReadTimeout(10 * time.Second)
		n, err := port.Read(buf)
		if err != nil {
			log.Printf("⚠️  [ASTM] Session read error: %v\n", err)
			return 0, false
		}
		if n == 0 {
			log.Println("⚠️  [ASTM] Session timed out — no data for 10s")
			return 0, false
		}
		return buf[0], true
	}

	ackFrame := func() bool {
		if _, err := port.Write([]byte{config.ACK}); err != nil {
			log.Println("❌ [ASTM] Failed to ACK frame:", err)
			return false
		}
		if lastEndByte == config.ETX {
			log.Printf("✅ [ASTM] Last frame ACKed (%d total)\n", frameCount)
		}
		return true
	}

	handleIdleByte := func(b byte) bool {
		switch b {
		case config.STX:
			frame.Reset()
			cur = inFrame
		case config.EOT:
			log.Println("📭 [ASTM] EOT received — transmission complete")
			if fullMessage.Len() > 0 {
				ProcessMessage(fullMessage.String())
			} else {
				log.Println("⚠️  [ASTM] EOT with no data collected")
			}
			return false
		case config.ENQ:
			log.Println("📥 [ASTM] Re-ENQ — sending ACK")
			port.Write([]byte{config.ACK})
			cur = idle
		default:
			log.Printf("[ASTM] Ignoring unexpected idle byte: 0x%02X\n", b)
		}
		return true
	}

	for {
		b, ok := readByte()
		if !ok {
			return
		}
		log.Printf("[ASTM] state=%d  byte=0x%02X (%s)\n", cur, b, byteDesc(b))

		switch cur {
		case idle:
			if !handleIdleByte(b) {
				return
			}

		case inFrame:
			if b == config.ETX || b == config.ETB {
				frameData := frame.String()
				if len(frameData) > 1 {
					data := frameData[1:]
					fullMessage.WriteString(data)
					frameCount++
					log.Printf("📦 [ASTM] Frame %d received (%d bytes)\n", frameCount, len(data))
					if config.DebugMode {
						log.Printf("   %q\n", data)
					}
				}
				lastEndByte = b
				tailCount = 0
				cur = tail
			} else {
				frame.WriteByte(b)
			}

		case tail:
			tailCount++

			if b == config.CR {
				if !ackFrame() {
					return
				}
				port.SetReadTimeout(200 * time.Millisecond)
				ln, _ := port.Read(buf)
				if ln > 0 && buf[0] != config.LF {
					if !handleIdleByte(buf[0]) {
						return
					}
				} else {
					cur = idle
				}

			} else if b == config.STX || b == config.EOT || b == config.ENQ || b == config.ETX || b == config.ETB {
				log.Printf("⚠️  [ASTM] Control byte 0x%02X in tail after %d bytes — ACKing and handling\n", b, tailCount)
				if !ackFrame() {
					return
				}
				if !handleIdleByte(b) {
					return
				}
			}
		}
	}
}

func byteDesc(b byte) string {
	switch b {
	case config.ENQ:
		return "ENQ"
	case config.ACK:
		return "ACK"
	case config.NAK:
		return "NAK"
	case config.STX:
		return "STX"
	case config.ETX:
		return "ETX"
	case config.ETB:
		return "ETB"
	case config.EOT:
		return "EOT"
	case config.CR:
		return "CR"
	case config.LF:
		return "LF"
	default:
		if b >= 32 && b <= 126 {
			return fmt.Sprintf("'%c'", b)
		}
		return "ctrl"
	}
}
