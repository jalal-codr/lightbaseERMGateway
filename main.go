package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

/*
CONTROL CHARACTERS
*/
const (
	// HL7 MLLP
	VT = 0x0B // Vertical Tab (Start Block)
	FS = 0x1C // File Separator (End Block)
	CR = 0x0D // Carriage Return
	LF = 0x0A // Line Feed
)

/*
CONFIGURATION - CHANGE THESE FOR YOUR SETUP
*/
const (
	// TCP/IP Configuration
	LISTEN_ADDRESS = "0.0.0.0:7007" // Listen on all interfaces, port 7007

	// Or if you need to CONNECT to the LIS (client mode):
	// LIS_ADDRESS = "192.168.1.100:7007" // IP and port of your LIS system

	// Server endpoint to forward results
	HL7_SERVER_ENDPOINT = "https://your-server.com/api/lis/hl7-results"

	// Debug mode
	DEBUG_MODE = true

	// Server mode: true = listen for connections, false = connect to LIS
	SERVER_MODE = true
)

/*
ENTRY POINT
*/
func main() {
	log.Println("🚀 Starting HL7 TCP/IP Listener")
	log.Println(strings.Repeat("=", 60))
	log.Printf("Mode: %s\n", getMode())
	log.Printf("Address: %s\n", LISTEN_ADDRESS)
	log.Println(strings.Repeat("=", 60))

	if SERVER_MODE {
		// Listen for incoming connections from LIS
		startServer()
	} else {
		// Connect to LIS as client
		startClient()
	}
}

func getMode() string {
	if SERVER_MODE {
		return "SERVER (waiting for LIS to connect)"
	}
	return "CLIENT (connecting to LIS)"
}

// ============================================================================
// SERVER MODE - Listen for connections from LIS
// ============================================================================

func startServer() {
	listener, err := net.Listen("tcp", LISTEN_ADDRESS)
	if err != nil {
		log.Fatal("❌ Failed to start server:", err)
	}
	defer listener.Close()

	log.Printf("✅ Server listening on %s\n", LISTEN_ADDRESS)
	log.Println("⏳ Waiting for LIS to connect...")
	log.Println(strings.Repeat("=", 60))

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("❌ Accept error:", err)
			continue
		}

		log.Printf("\n🔔 NEW CONNECTION from %s\n", conn.RemoteAddr())
		log.Println(strings.Repeat("-", 60))

		// Handle connection in goroutine to support multiple simultaneous connections
		go handleConnection(conn)
	}
}

// ============================================================================
// CLIENT MODE - Connect to LIS
// ============================================================================

func startClient() {
	for {
		log.Printf("🔌 Connecting to LIS at %s...\n", LISTEN_ADDRESS)

		conn, err := net.Dial("tcp", LISTEN_ADDRESS)
		if err != nil {
			log.Println("❌ Connection failed:", err)
			log.Println("⏳ Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("✅ Connected to %s\n", conn.RemoteAddr())
		log.Println(strings.Repeat("=", 60))

		handleConnection(conn)

		log.Println("⚠️ Connection closed, reconnecting...")
		time.Sleep(2 * time.Second)
	}
}

// ============================================================================
// CONNECTION HANDLER - Process HL7 messages
// ============================================================================

func handleConnection(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	var messageBuffer bytes.Buffer
	inMessage := false
	byteCount := 0

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err != io.EOF {
				log.Println("❌ Read error:", err)
			} else {
				log.Println("📡 Connection closed by remote")
			}
			return
		}

		byteCount++

		if DEBUG_MODE {
			log.Printf("Byte %d: 0x%02X (%s)\n", byteCount, b, byteDescription(b))
		}

		switch b {
		case VT:
			// Start of HL7 message
			inMessage = true
			messageBuffer.Reset()
			log.Println("\n➡️ [HL7] Message Start (VT received)")
			log.Println(strings.Repeat("-", 60))

		case FS:
			// End of HL7 message
			if inMessage {
				inMessage = false
				log.Println("⬅️ [HL7] Message End (FS received)")
				processHL7Message(messageBuffer.String(), conn)
				messageBuffer.Reset()
			}

		case CR:
			if inMessage {
				messageBuffer.WriteByte(b)
			}

		case LF:
			// Some systems send CRLF, usually ignore LF
			if inMessage && DEBUG_MODE {
				log.Println("   [LF received, ignoring]")
			}

		default:
			if inMessage {
				messageBuffer.WriteByte(b)
			}
		}
	}
}

func byteDescription(b byte) string {
	switch b {
	case VT:
		return "VT (HL7 Start)"
	case FS:
		return "FS (HL7 End)"
	case CR:
		return "CR"
	case LF:
		return "LF"
	default:
		if b >= 32 && b <= 126 {
			return fmt.Sprintf("'%c'", b)
		}
		return "data"
	}
}

// ============================================================================
// HL7 MESSAGE PROCESSING
// ============================================================================

func processHL7Message(message string, conn net.Conn) {
	log.Println("\n📦 [HL7] MESSAGE RECEIVED")
	log.Println(strings.Repeat("=", 60))

	if DEBUG_MODE {
		log.Println("Raw Message:")
		log.Println(message)
		log.Println(strings.Repeat("-", 60))

		// Show hex dump
		log.Println("Hex Dump:")
		log.Println(hex.Dump([]byte(message)))
		log.Println(strings.Repeat("-", 60))
	}

	// Parse HL7 message
	results := parseHL7(message)

	// Send ACK back
	ack := generateHL7ACK(message)
	if ack != "" {
		ackBytes := []byte{VT}
		ackBytes = append(ackBytes, []byte(ack)...)
		ackBytes = append(ackBytes, FS, CR)

		_, err := conn.Write(ackBytes)
		if err != nil {
			log.Println("❌ Error sending ACK:", err)
		} else {
			log.Println("✅ [HL7] ACK sent")
			if DEBUG_MODE {
				log.Printf("ACK Content:\n%s\n", ack)
			}
		}
	} else {
		log.Println("⚠️ Could not generate ACK - invalid message format")
	}

	// Forward results to server
	if len(results) > 0 {
		log.Printf("✅ Parsed %d result(s)\n", len(results))
		sendHL7ToServer(results)
	} else {
		log.Println("⚠️ No OBX results found in message")
	}

	log.Println(strings.Repeat("=", 60) + "\n")
}

func parseHL7(message string) []map[string]interface{} {
	// Handle both CR and CRLF line endings
	message = strings.ReplaceAll(message, "\r\n", "\r")
	segments := strings.Split(message, string(CR))

	results := []map[string]interface{}{}
	var patientID, patientName, accessionNumber, messageControlID string

	log.Printf("Found %d segments:\n", len(segments))

	for i, segment := range segments {
		segment = strings.TrimSpace(segment)
		if len(segment) == 0 {
			continue
		}

		fields := strings.Split(segment, "|")
		if len(fields) == 0 {
			continue
		}

		segmentType := fields[0]
		log.Printf("  %d. %s (%d fields)\n", i+1, segmentType, len(fields))

		switch segmentType {
		case "MSH":
			messageControlID = getHL7Field(fields, 9)
			if DEBUG_MODE {
				log.Printf("     Message ID: %s\n", messageControlID)
			}

		case "PID":
			patientID = getHL7Field(fields, 3)
			patientName = getHL7Field(fields, 5)
			if DEBUG_MODE {
				log.Printf("     Patient: %s (ID: %s)\n", patientName, patientID)
			}

		case "OBR":
			accessionNumber = getHL7Field(fields, 2)
			if DEBUG_MODE {
				log.Printf("     Accession: %s\n", accessionNumber)
			}

		case "OBX":
			testCode := parseHL7Component(getHL7Field(fields, 3), 0)
			testName := parseHL7Component(getHL7Field(fields, 3), 1)
			value := getHL7Field(fields, 5)
			units := getHL7Field(fields, 6)

			if DEBUG_MODE {
				log.Printf("     Test: %s (%s) = %s %s\n", testName, testCode, value, units)
			}

			result := map[string]interface{}{
				"patient_id":       patientID,
				"patient_name":     patientName,
				"accession_number": accessionNumber,
				"message_id":       messageControlID,
				"observation_id":   getHL7Field(fields, 1),
				"value_type":       getHL7Field(fields, 2),
				"test_code":        testCode,
				"test_name":        testName,
				"value":            value,
				"units":            units,
				"reference_range":  getHL7Field(fields, 7),
				"abnormal_flags":   getHL7Field(fields, 8),
				"result_status":    getHL7Field(fields, 11),
				"timestamp":        parseHL7DateTime(getHL7Field(fields, 14)),
			}
			results = append(results, result)
		}
	}

	return results
}

func getHL7Field(fields []string, index int) string {
	if index >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[index])
}

func parseHL7Component(field string, componentIndex int) string {
	components := strings.Split(field, "^")
	if componentIndex >= len(components) {
		return ""
	}
	return strings.TrimSpace(components[componentIndex])
}

func parseHL7DateTime(hl7DateTime string) string {
	hl7DateTime = strings.TrimSpace(hl7DateTime)
	if len(hl7DateTime) < 8 {
		return time.Now().Format(time.RFC3339)
	}

	layout := "20060102150405"
	if len(hl7DateTime) >= 14 {
		t, err := time.Parse(layout, hl7DateTime[:14])
		if err == nil {
			return t.Format(time.RFC3339)
		}
	}

	layout = "20060102"
	t, err := time.Parse(layout, hl7DateTime[:8])
	if err == nil {
		return t.Format(time.RFC3339)
	}

	return time.Now().Format(time.RFC3339)
}

func generateHL7ACK(originalMessage string) string {
	originalMessage = strings.ReplaceAll(originalMessage, "\r\n", "\r")
	segments := strings.Split(originalMessage, string(CR))

	var mshFields []string

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if strings.HasPrefix(segment, "MSH") {
			mshFields = strings.Split(segment, "|")
			break
		}
	}

	if len(mshFields) < 10 {
		if DEBUG_MODE {
			log.Printf("Invalid MSH - only %d fields\n", len(mshFields))
		}
		return ""
	}

	fieldSeparator := "|"
	encodingChars := getHL7Field(mshFields, 1)
	sendingApp := getHL7Field(mshFields, 2)
	sendingFacility := getHL7Field(mshFields, 3)
	receivingApp := getHL7Field(mshFields, 4)
	receivingFacility := getHL7Field(mshFields, 5)
	messageControlID := getHL7Field(mshFields, 9)

	timestamp := time.Now().Format("20060102150405")

	// Build ACK
	ack := fmt.Sprintf("MSH%s%s%s%s%s%s%s%s%s%sACK%s%s%sAL%s",
		fieldSeparator,
		encodingChars,
		fieldSeparator,
		receivingApp, // Swap sender/receiver
		fieldSeparator,
		receivingFacility,
		fieldSeparator,
		sendingApp,
		fieldSeparator,
		sendingFacility,
		fieldSeparator,
		timestamp,
		fieldSeparator,
		fieldSeparator,
		fieldSeparator,
	)
	ack += string(CR)

	ack += fmt.Sprintf("MSA%sAA%s%s",
		fieldSeparator,
		fieldSeparator,
		messageControlID,
	)

	return ack
}

func sendHL7ToServer(results []map[string]interface{}) {
	log.Println("\n📤 Forwarding to server...")

	if DEBUG_MODE {
		for i, r := range results {
			log.Printf("  Result %d: %+v\n", i+1, r)
		}
	}

	payload, err := json.Marshal(results)
	if err != nil {
		log.Println("❌ JSON error:", err)
		return
	}

	req, err := http.NewRequest("POST", HL7_SERVER_ENDPOINT, bytes.NewBuffer(payload))
	if err != nil {
		log.Println("❌ Request error:", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("❌ Server unreachable:", err)
		return
	}
	defer resp.Body.Close()

	log.Println("✅ Results forwarded:", resp.Status)
}
