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

	"go.bug.st/serial"
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
ASTM CONTROL CHARACTERS
*/
const (
	ENQ = 0x05 // Enquiry   — instrument wants to send
	ACK = 0x06 // Acknowledge
	NAK = 0x15 // Negative Acknowledge
	STX = 0x02 // Start of Text
	ETX = 0x03 // End of Text (last frame)
	ETB = 0x17 // End of Transmission Block (intermediate frame)
	EOT = 0x04 // End of Transmission
)

/*
CONFIGURATION
*/
const (
	PC_IP           = "192.168.1.193"
	LISTEN_PORT     = "7007"
	DEBUG_MODE      = true
	LOG_TO_TERMINAL = true

	ASTM_COM_PORT  = "COM1"
	ASTM_BAUD_RATE = 115200
	ASTM_TCP_PORT  = "5000" // TCP port the instrument connects to
)

// astmPort is satisfied by both serial.Port and tcpASTMConn,
// allowing handleASTMPort / handleASTMSession to work over either transport.
type astmPort interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	SetReadTimeout(t time.Duration) error
}

// tcpASTMConn wraps a net.Conn so it satisfies astmPort.
// SetReadTimeout translates to an absolute deadline, and Read normalises
// timeout errors to (0, nil) to match serial-port behaviour.
type tcpASTMConn struct{ conn net.Conn }

func (t *tcpASTMConn) Read(b []byte) (int, error) {
	n, err := t.conn.Read(b)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, nil
		}
		return n, err
	}
	return n, nil
}
func (t *tcpASTMConn) Write(b []byte) (int, error) { return t.conn.Write(b) }
func (t *tcpASTMConn) SetReadTimeout(d time.Duration) error {
	return t.conn.SetReadDeadline(time.Now().Add(d))
}

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

	// Start ASTM serial listener on COM1 (non-blocking)
	go startASTMListener()

	// Start ASTM TCP listener (instrument connects over Ethernet)
	go startASTMTCPListener()

	// Start HL7 TCP server (blocks)
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
			// Connection closed — determine if this was a ping
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

		if DEBUG_MODE && byteCount <= 100 {
			log.Printf("Byte %d: 0x%02X (%s)\n", byteCount, b, byteDescription(b))
		}

		switch b {
		case VT:
			inMessage = true
			messageBuffer.Reset()
			pingBuffer.Reset() // clear ping buffer — HL7 framing has started
			log.Println("\n➡️ [HL7] Message Start (VT received)")

		case FS:
			if inMessage {
				inMessage = false
				messagesReceived++
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
			} else {
				// Bytes outside HL7 framing — likely a ping/probe
				pingBuffer.WriteByte(b)
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

// ============================================================================
// ASTM TCP LISTENER — instruments that connect over Ethernet
// ============================================================================

func startASTMTCPListener() {
	addr := "0.0.0.0:" + ASTM_TCP_PORT
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
			handleASTMPort(&tcpASTMConn{conn: c})
			log.Printf("🔌 [ASTM-TCP] Instrument disconnected: %s\n", c.RemoteAddr())
		}(conn)
	}
}

// ============================================================================
// ASTM SERIAL LISTENER — COM1
// ============================================================================

func startASTMListener() {
	mode := &serial.Mode{
		BaudRate: ASTM_BAUD_RATE,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	log.Printf("📡 [ASTM] Opening %s at %d baud...\n", ASTM_COM_PORT, ASTM_BAUD_RATE)

	for {
		port, err := serial.Open(ASTM_COM_PORT, mode)
		if err != nil {
			log.Printf("❌ [ASTM] Could not open %s: %v — retrying in 5s\n", ASTM_COM_PORT, err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("✅ [ASTM] %s open — waiting for ENQ from instrument...\n", ASTM_COM_PORT)
		handleASTMPort(port)
		port.Close()
		log.Printf("⚠️  [ASTM] Session ended, reopening %s...\n", ASTM_COM_PORT)
		time.Sleep(1 * time.Second)
	}
}

func handleASTMPort(port astmPort) {
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
		log.Printf("[ASTM] Received: 0x%02X (%s)\n", b, astmByteDesc(b))

		if b == ENQ {
			log.Println("\n📥 [ASTM] ENQ received — sending ACK")
			if _, err := port.Write([]byte{ACK}); err != nil {
				log.Println("❌ [ASTM] Failed to send ACK:", err)
				return
			}
			handleASTMSession(port)
		}
	}
}

func handleASTMSession(port astmPort) {
	type sState int
	const (
		sIdle    sState = iota
		sInFrame
		sTail
	)

	var fullMessage strings.Builder
	var frame bytes.Buffer
	frameCount := 0
	tailCount := 0
	lastEndByte := byte(0)
	cur := sIdle
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
		if _, err := port.Write([]byte{ACK}); err != nil {
			log.Println("❌ [ASTM] Failed to ACK frame:", err)
			return false
		}
		if lastEndByte == ETX {
			log.Printf("✅ [ASTM] Last frame ACKed (%d total)\n", frameCount)
		}
		return true
	}

	handleIdleByte := func(b byte) bool {
		switch b {
		case STX:
			frame.Reset()
			cur = sInFrame
		case EOT:
			log.Println("📭 [ASTM] EOT received — transmission complete")
			if fullMessage.Len() > 0 {
				processASTMMessage(fullMessage.String())
			} else {
				log.Println("⚠️  [ASTM] EOT with no data collected")
			}
			return false
		case ENQ:
			log.Println("📥 [ASTM] Re-ENQ — sending ACK")
			port.Write([]byte{ACK})
			cur = sIdle
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
		log.Printf("[ASTM] state=%d  byte=0x%02X (%s)\n", cur, b, astmByteDesc(b))

		switch cur {

		case sIdle:
			if !handleIdleByte(b) {
				return
			}

		case sInFrame:
			if b == ETX || b == ETB {
				frameData := frame.String()
				if len(frameData) > 1 {
					data := frameData[1:]
					fullMessage.WriteString(data)
					frameCount++
					log.Printf("📦 [ASTM] Frame %d received (%d bytes)\n", frameCount, len(data))
					if DEBUG_MODE {
						log.Printf("   %q\n", data)
					}
				}
				lastEndByte = b
				tailCount = 0
				cur = sTail
			} else {
				frame.WriteByte(b)
			}

		case sTail:
			tailCount++

			if b == CR {
				if !ackFrame() {
					return
				}
				port.SetReadTimeout(200 * time.Millisecond)
				ln, _ := port.Read(buf)
				if ln > 0 && buf[0] != LF {
					if !handleIdleByte(buf[0]) {
						return
					}
				} else {
					cur = sIdle
				}

			} else if b == STX || b == EOT || b == ENQ || b == ETX || b == ETB {
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

// ============================================================================
// ASTM MESSAGE PROCESSING (E1394)
// ============================================================================

func processASTMMessage(message string) {
	log.Println("\n📦 [ASTM] MESSAGE RECEIVED")
	if DEBUG_MODE {
		log.Println("Raw ASTM:\n", message)
		log.Println(strings.Repeat("-", 60))
	}

	records := strings.Split(message, string(CR))
	results := []map[string]interface{}{}

	var patientID, patientName, orderID string

	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.Split(record, "|")
		if len(fields) == 0 {
			continue
		}
		recordType := fields[0]

		switch recordType {
		case "H":
			log.Println("[ASTM] Header record")
		case "P":
			patientID = getASTMField(fields, 3)
			patientName = getASTMField(fields, 5)
			log.Printf("[ASTM] Patient: ID=%s  Name=%s\n", patientID, patientName)
		case "O":
			orderID = getASTMField(fields, 2)
			log.Printf("[ASTM] Order: ID=%s\n", orderID)
		case "R":
			result := map[string]interface{}{
				"patient_id":      patientID,
				"patient_name":    patientName,
				"order_id":        orderID,
				"test_code":       parseASTMComponent(getASTMField(fields, 2), 3),
				"test_name":       parseASTMComponent(getASTMField(fields, 2), 4),
				"value":           getASTMField(fields, 3),
				"units":           getASTMField(fields, 4),
				"reference_range": getASTMField(fields, 5),
				"abnormal_flags":  getASTMField(fields, 6),
				"result_status":   getASTMField(fields, 8),
				"timestamp":       parseHL7DateTime(getASTMField(fields, 12)),
				"protocol":        "ASTM",
			}
			results = append(results, result)
		case "L":
			log.Println("[ASTM] Terminator record")
		}
	}

	if LOG_TO_TERMINAL && len(results) > 0 {
		logResultsToTerminal(results)
	} else if len(results) == 0 {
		log.Println("⚠️  [ASTM] No R (result) records found in message")
	}
}

func getASTMField(fields []string, index int) string {
	if index >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[index])
}

func parseASTMComponent(field string, componentIndex int) string {
	components := strings.Split(field, "^")
	if componentIndex >= len(components) {
		return ""
	}
	return strings.TrimSpace(components[componentIndex])
}

func astmByteDesc(b byte) string {
	switch b {
	case ENQ:
		return "ENQ"
	case ACK:
		return "ACK"
	case NAK:
		return "NAK"
	case STX:
		return "STX"
	case ETX:
		return "ETX"
	case ETB:
		return "ETB"
	case EOT:
		return "EOT"
	case CR:
		return "CR"
	case LF:
		return "LF"
	default:
		if b >= 32 && b <= 126 {
			return fmt.Sprintf("'%c'", b)
		}
		return "ctrl"
	}
}
