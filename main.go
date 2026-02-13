package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

/*
CONTROL CHARACTERS
*/
const (
	VT = 0x0B // Start Block
	FS = 0x1C // End Block
	CR = 0x0D // Carriage Return
	LF = 0x0A // Line Feed
)

/*
CONFIGURATION
*/
const (
	PC_IP           = "192.168.110.193"
	LISTEN_PORT     = "7007"
	DEBUG_MODE      = true
	LOG_TO_TERMINAL = true
)

/*
ENTRY POINT
*/
func main() {
	log.Println("🚀 Starting HL7 TCP/IP Server (Listening for LIS connections)")
	log.Println(strings.Repeat("=", 60))
	fullAddress := PC_IP + ":" + LISTEN_PORT
	log.Printf("Listening on %s for incoming LIS connections...\n", fullAddress)

	// Print local IP addresses
	printLocalIPs()

	// Start server
	startServer(fullAddress)
}

func printLocalIPs() {
	log.Println("\n📡 This Computer's IP Addresses:")
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Println("   Could not get local IPs:", err)
		return
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				log.Printf("   ℹ️  %s\n", ipnet.IP.String())
			}
		}
	}
	log.Println()
}

// ============================================================================
// TCP SERVER - LISTEN FOR LIS
// ============================================================================

func startServer(address string) {
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

// ============================================================================
// CONNECTION HANDLER - Process HL7 messages
// ============================================================================

func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	var messageBuffer bytes.Buffer
	inMessage := false
	byteCount := 0
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
			if err != nil {
				log.Println("❌ Read error:", err)
			}
			return
		}

		lastActivity = time.Now()
		byteCount++

		if DEBUG_MODE && byteCount <= 100 {
			log.Printf("Byte %d: 0x%02X (%s)\n", byteCount, b, byteDescription(b))
		}

		switch b {
		case VT:
			inMessage = true
			messageBuffer.Reset()
			log.Println("\n➡️ [HL7] Message Start (VT received)")

		case FS:
			if inMessage {
				inMessage = false
				log.Println("⬅️ [HL7] Message End (FS received)")
				processHL7Message(messageBuffer.String(), conn)
				messageBuffer.Reset()
				byteCount = 0
			}

		case CR:
			if inMessage {
				messageBuffer.WriteByte(b)
			}

		case LF:
			if inMessage && DEBUG_MODE && byteCount <= 100 {
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
	if DEBUG_MODE {
		log.Println("Raw Message:\n", message)
		log.Println(strings.Repeat("-", 60))
		log.Println("Hex Dump:\n", hex.Dump([]byte(message)))
	}

	results := parseHL7(message)

	// Send ACK back to LIS
	ack := generateHL7ACK(message)
	if ack != "" {
		ackBytes := []byte{VT}
		ackBytes = append(ackBytes, []byte(ack)...)
		ackBytes = append(ackBytes, FS, CR)

		_, err := conn.Write(ackBytes)
		if err != nil {
			log.Println("❌ Error sending ACK:", err)
		} else {
			log.Println("✅ [HL7] ACK sent to LIS")
		}
	} else {
		log.Println("⚠️ Could not generate ACK - invalid message")
	}

	if LOG_TO_TERMINAL && len(results) > 0 {
		logResultsToTerminal(results)
	}
}

// ============================================================================
// HL7 PARSING & ACK GENERATION (same as your previous logic)
// ============================================================================

func parseHL7(message string) []map[string]interface{} {
	message = strings.ReplaceAll(message, "\r\n", "\r")
	segments := strings.Split(message, string(CR))

	results := []map[string]interface{}{}
	var patientID, patientName, accessionNumber, messageControlID string

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		fields := strings.Split(segment, "|")
		if len(fields) == 0 {
			continue
		}
		segmentType := fields[0]

		switch segmentType {
		case "MSH":
			messageControlID = getHL7Field(fields, 9)
		case "PID":
			patientID = getHL7Field(fields, 3)
			patientName = getHL7Field(fields, 5)
		case "OBR":
			accessionNumber = getHL7Field(fields, 2)
		case "OBX":
			result := map[string]interface{}{
				"patient_id":       patientID,
				"patient_name":     patientName,
				"accession_number": accessionNumber,
				"message_id":       messageControlID,
				"observation_id":   getHL7Field(fields, 1),
				"value_type":       getHL7Field(fields, 2),
				"test_code":        parseHL7Component(getHL7Field(fields, 3), 0),
				"test_name":        parseHL7Component(getHL7Field(fields, 3), 1),
				"value":            getHL7Field(fields, 5),
				"units":            getHL7Field(fields, 6),
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

	ack := fmt.Sprintf("MSH%s%s%s%s%s%s%s%s%s%sACK%s%s%sAL%s",
		fieldSeparator,
		encodingChars,
		fieldSeparator,
		receivingApp,
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

// ============================================================================
// LOGGING RESULTS
// ============================================================================

func logResultsToTerminal(results []map[string]interface{}) {
	log.Println("\n" + strings.Repeat("*", 60))
	log.Println("*** LAB RESULTS - TERMINAL OUTPUT ***")
	log.Println(strings.Repeat("*", 60))

	for i, result := range results {
		log.Printf("\n📋 Result #%d:\n", i+1)
		log.Println(strings.Repeat("-", 60))
		log.Println("👤 PATIENT INFORMATION:")
		log.Printf("   Patient ID:       %v\n", result["patient_id"])
		log.Printf("   Patient Name:     %v\n", result["patient_name"])
		log.Printf("   Accession Number: %v\n", result["accession_number"])
		log.Println("\n🧪 TEST INFORMATION:")
		log.Printf("   Test Code:        %v\n", result["test_code"])
		log.Printf("   Test Name:        %v\n", result["test_name"])
		log.Printf("   Value:            %v %v\n", result["value"], result["units"])
		log.Printf("   Reference Range:  %v\n", result["reference_range"])
		log.Printf("   Abnormal Flags:   %v\n", result["abnormal_flags"])
		log.Printf("   Result Status:    %v\n", result["result_status"])
		log.Println("\n📨 MESSAGE INFORMATION:")
		log.Printf("   Message ID:       %v\n", result["message_id"])
		log.Printf("   Observation ID:   %v\n", result["observation_id"])
		log.Printf("   Value Type:       %v\n", result["value_type"])
		log.Printf("   Timestamp:        %v\n", result["timestamp"])
		log.Println(strings.Repeat("-", 60))
	}

	log.Println("\n📄 JSON FORMAT:")
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err == nil {
		log.Println(string(jsonData))
	}
	log.Println(strings.Repeat("*", 60))
}
