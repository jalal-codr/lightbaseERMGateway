package hl7

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"
	"lightbaseEMRProxy/internal/logger"
)

// StartServer starts the HL7 TCP server
func StartServer(address string) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal("❌ Failed to start server:", err)
	}
	defer ln.Close()

	log.Println("✅ HL7 Server is listening... Waiting for LIS to connect.")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("❌ Accept error:", err)
			continue
		}
		log.Printf("🔌 LIS Connected: %s -> %s\n", conn.RemoteAddr(), conn.LocalAddr())
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	var messageBuffer bytes.Buffer
	var pingBuffer bytes.Buffer
	inMessage := false
	byteCount := 0
	messagesReceived := 0
	lastActivity := time.Now()

	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	log.Println("\n📊 Connection established, listening for HL7 data...")

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if time.Since(lastActivity) > 30*time.Second {
					log.Println("\n⏰ No data received for 30 seconds")
				}
				conn.SetReadDeadline(time.Now().Add(30 * time.Second))
				continue
			}
			if messagesReceived == 0 {
				if pingBuffer.Len() > 0 {
					log.Printf("\n🏓 [PING] LIS sent raw bytes (no HL7 framing): %q\n", pingBuffer.String())
				} else {
					log.Println("\n🏓 [PING] LIS connected and disconnected without sending HL7 data")
				}
			}
			log.Println("🔌 Connection closed:", err)
			return
		}

		lastActivity = time.Now()
		byteCount++

		if config.DebugMode && byteCount <= 100 {
			log.Printf("Byte %d: 0x%02X (%s)\n", byteCount, b, byteDescription(b))
		}

		switch b {
		case config.VT:
			inMessage = true
			messageBuffer.Reset()
			pingBuffer.Reset()
			log.Println("\n➡️ [HL7] Message Start (VT received)")

		case config.FS:
			if inMessage {
				inMessage = false
				messagesReceived++
				log.Println("⬅️ [HL7] Message End (FS received)")
				processMessage(messageBuffer.String(), conn)
				messageBuffer.Reset()
				byteCount = 0
			}

		case config.CR:
			if inMessage {
				messageBuffer.WriteByte(b)
			}

		case config.LF:
			if inMessage && config.DebugMode && byteCount <= 100 {
				log.Println("   [LF received, ignoring]")
			}

		default:
			if inMessage {
				messageBuffer.WriteByte(b)
			} else {
				pingBuffer.WriteByte(b)
			}
		}
	}
}

func processMessage(message string, conn net.Conn) {
	log.Println("\n📦 [HL7] MESSAGE RECEIVED")
	if config.DebugMode {
		log.Println("Raw Message:\n", message)
		log.Println(strings.Repeat("-", 60))
		log.Println("Hex Dump:\n", hex.Dump([]byte(message)))
	}

	results := ParseMessage(message)

	ack := GenerateACK(message)
	if ack != "" {
		ackBytes := []byte{config.VT}
		ackBytes = append(ackBytes, []byte(ack)...)
		ackBytes = append(ackBytes, config.FS, config.CR)

		_, err := conn.Write(ackBytes)
		if err != nil {
			log.Println("❌ Error sending ACK:", err)
		} else {
			log.Println("✅ [HL7] ACK sent to LIS")
		}
	} else {
		log.Println("⚠️ Could not generate ACK - invalid message")
	}

	if config.LogToTerminal && len(results) > 0 {
		logger.LogResults(results)
	}
}

func byteDescription(b byte) string {
	switch b {
	case config.VT:
		return "VT (HL7 Start)"
	case config.FS:
		return "FS (HL7 End)"
	case config.CR:
		return "CR"
	case config.LF:
		return "LF"
	default:
		if b >= 32 && b <= 126 {
			return fmt.Sprintf("'%c'", b)
		}
		return "data"
	}
}
