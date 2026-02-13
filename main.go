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
CONFIGURATION - EXTRACTED FROM YOUR DEVICE
*/
const (
	// LIS DEVICE IP AND PORT - From your device configuration screen
	LIS_DEVICE_IP      = "192.168.1.10"
	LIS_DEVICE_PORT    = "7007"
	LIS_DEVICE_ADDRESS = LIS_DEVICE_IP + ":" + LIS_DEVICE_PORT

	// Debug mode
	DEBUG_MODE = true

	// Client mode: Connect TO the LIS device
	SERVER_MODE = false

	// Log results to terminal
	LOG_TO_TERMINAL = true
)

/*
ENTRY POINT
*/
func main() {
	log.Println("🚀 Starting HL7 TCP/IP Client")
	log.Println(strings.Repeat("=", 60))
	log.Printf("Mode: CLIENT (connecting TO LIS device)\n")
	log.Printf("LIS Device IP: %s\n", LIS_DEVICE_IP)
	log.Printf("LIS Device Port: %s\n", LIS_DEVICE_PORT)
	log.Printf("Full Address: %s\n", LIS_DEVICE_ADDRESS)
	log.Printf("Protocol: HL7 MLLP\n")
	log.Printf("Results will be: LOGGED TO TERMINAL\n")

	// Print local IP addresses for reference
	printLocalIPs()

	log.Println(strings.Repeat("=", 60))
	log.Println("⏳ Starting connection attempts...")

	// Connect to LIS device
	startClient()
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
// CLIENT MODE - Connect to LIS Device at 192.168.1.10:7007
// ============================================================================

func startClient() {
	retryCount := 0

	for {
		retryCount++
		log.Printf("\n🔌 Attempt #%d: Connecting to LIS device at %s...\n", retryCount, LIS_DEVICE_ADDRESS)

		conn, err := net.DialTimeout("tcp", LIS_DEVICE_ADDRESS, 10*time.Second)
		if err != nil {
			log.Println("❌ Connection failed:", err)

			if retryCount == 1 {
				log.Println("\n⚠️  TROUBLESHOOTING TIPS:")
				log.Println("   1. Make sure the LIS device is powered on")
				log.Println("   2. Verify both devices are on the same network (192.168.1.x)")
				log.Println("   3. Check that HL7 communication is enabled on the device")
				log.Printf("   4. Try pinging the device first: ping %s\n", LIS_DEVICE_IP)
				log.Println("   5. Check if Windows Firewall is blocking the connection")
				log.Println("   6. Verify the device is set to 'Auto Comm' mode (if applicable)")
				log.Println()
			}

			log.Println("⏳ Retrying in 5 seconds...")
			time.Sleep(5 * time.Second)
			continue
		}

		retryCount = 0 // Reset on successful connection
		log.Printf("✅ Connected to LIS device at %s\n", conn.RemoteAddr())
		log.Printf("   Local connection from: %s\n", conn.LocalAddr())
		log.Println(strings.Repeat("=", 60))

		handleConnection(conn)

		log.Println("\n⚠️ Connection closed by device, reconnecting in 2 seconds...")
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

	log.Println("\n📊 Connection established, listening for HL7 data from device...")
	log.Println("💡 TIP: Run a test on the device to trigger result transmission")
	log.Println(strings.Repeat("-", 60))

	// Set read timeout to detect if connection is idle
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	lastActivity := time.Now()

	for {
		b, err := reader.ReadByte()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Check if we've had recent activity
				if time.Since(lastActivity) > 30*time.Second {
					log.Println("\n⏰ No data received for 30 seconds")
					log.Println("💡 The device is connected but not sending data")
					log.Println("   Try running a test or checking the 'Auto Comm' setting")
				}
				// Reset timeout and continue
				conn.SetReadDeadline(time.Now().Add(30 * time.Second))
				continue
			}

			if err != io.EOF {
				log.Println("❌ Read error:", err)
			} else {
				log.Println("📡 Connection closed by LIS device")
			}
			return
		}

		lastActivity = time.Now()
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		byteCount++

		// Log first few bytes to confirm data reception
		if byteCount == 1 {
			log.Println("\n✅ Data received from device!")
		}

		if DEBUG_MODE && byteCount <= 100 { // Limit debug output for first 100 bytes
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
				byteCount = 0 // Reset for next message
			}

		case CR:
			if inMessage {
				messageBuffer.WriteByte(b)
			}

		case LF:
			// Some systems send CRLF, usually ignore LF
			if inMessage && DEBUG_MODE && byteCount <= 100 {
				log.Println("   [LF received, ignoring]")
			}

		default:
			if inMessage {
				messageBuffer.WriteByte(b)
			} else if DEBUG_MODE && byteCount <= 20 {
				// Data received outside of message boundaries
				log.Printf("⚠️  Unexpected byte outside message: 0x%02X (%s)\n", b, byteDescription(b))
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
	log.Println("\n📦 [HL7] MESSAGE RECEIVED FROM DEVICE")
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

	// Send ACK back to device
	ack := generateHL7ACK(message)
	if ack != "" {
		ackBytes := []byte{VT}
		ackBytes = append(ackBytes, []byte(ack)...)
		ackBytes = append(ackBytes, FS, CR)

		_, err := conn.Write(ackBytes)
		if err != nil {
			log.Println("❌ Error sending ACK:", err)
		} else {
			log.Println("✅ [HL7] ACK sent back to device")
			if DEBUG_MODE {
				log.Printf("ACK Content:\n%s\n", ack)
			}
		}
	} else {
		log.Println("⚠️ Could not generate ACK - invalid message format")
	}

	// Log results to terminal
	if len(results) > 0 {
		log.Printf("\n✅ Parsed %d result(s) from message\n", len(results))
		logResultsToTerminal(results)
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

// ============================================================================
// TERMINAL LOGGING - Log results in a nice formatted way
// ============================================================================

func logResultsToTerminal(results []map[string]interface{}) {
	log.Println("\n" + strings.Repeat("*", 60))
	log.Println("*** LAB RESULTS - TERMINAL OUTPUT ***")
	log.Println(strings.Repeat("*", 60))

	for i, result := range results {
		log.Printf("\n📋 Result #%d:\n", i+1)
		log.Println(strings.Repeat("-", 60))

		// Patient Information
		log.Println("👤 PATIENT INFORMATION:")
		log.Printf("   Patient ID:       %v\n", result["patient_id"])
		log.Printf("   Patient Name:     %v\n", result["patient_name"])
		log.Printf("   Accession Number: %v\n", result["accession_number"])

		// Test Information
		log.Println("\n🧪 TEST INFORMATION:")
		log.Printf("   Test Code:        %v\n", result["test_code"])
		log.Printf("   Test Name:        %v\n", result["test_name"])
		log.Printf("   Value:            %v %v\n", result["value"], result["units"])
		log.Printf("   Reference Range:  %v\n", result["reference_range"])
		log.Printf("   Abnormal Flags:   %v\n", result["abnormal_flags"])
		log.Printf("   Result Status:    %v\n", result["result_status"])

		// Message Information
		log.Println("\n📨 MESSAGE INFORMATION:")
		log.Printf("   Message ID:       %v\n", result["message_id"])
		log.Printf("   Observation ID:   %v\n", result["observation_id"])
		log.Printf("   Value Type:       %v\n", result["value_type"])
		log.Printf("   Timestamp:        %v\n", result["timestamp"])

		log.Println(strings.Repeat("-", 60))
	}

	// Also print as JSON for easy copy/paste
	log.Println("\n📄 JSON FORMAT (for API integration):")
	log.Println(strings.Repeat("-", 60))
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Println("❌ Error formatting JSON:", err)
	} else {
		log.Println(string(jsonData))
	}

	log.Println(strings.Repeat("*", 60))
	log.Printf("✅ Total Results Logged: %d\n", len(results))
	log.Println(strings.Repeat("*", 60) + "\n")
}
