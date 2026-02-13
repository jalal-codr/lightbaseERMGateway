package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"go.bug.st/serial"
)

/*
CONTROL CHARACTERS
*/
const (
	// ASTM
	ENQ = 0x05
	ACK = 0x06
	NAK = 0x15
	EOT = 0x04
	STX = 0x02
	ETX = 0x03
	ETB = 0x17
	CR  = 0x0D
	LF  = 0x0A

	// HL7 MLLP
	VT = 0x0B // Vertical Tab (Start Block)
	FS = 0x1C // File Separator (End Block)
)

/*
CONFIGURATION - CHANGE THESE FOR YOUR SETUP
*/
const (
	// ASTM Configuration
	ASTM_COM_PORT        = "COM1"
	ASTM_BAUD_RATE       = 9600
	ASTM_SERVER_ENDPOINT = "https://your-server.com/api/lis/astm-results"

	// HL7 Configuration
	HL7_COM_PORT        = "COM1" // SET TO SAME PORT AS ASTM IF SHARING, OR "" TO DISABLE
	HL7_BAUD_RATE       = 9600
	HL7_SERVER_ENDPOINT = "https://your-server.com/api/lis/hl7-results"

	// Protocol Configuration
	ENABLE_ASTM = true
	ENABLE_HL7  = true

	// If both protocols share the same port, they will run on a single listener
	// Otherwise they run on separate ports

	// Debug mode - set to true to see detailed logs
	DEBUG_MODE = true
)

/*
ENTRY POINT - Runs protocols based on configuration
*/
func main() {
	log.Println("🚀 Starting Multi-Protocol LIS Application")
	log.Println(strings.Repeat("=", 50))

	// Check if protocols share the same port
	samePort := (ASTM_COM_PORT == HL7_COM_PORT) && ENABLE_ASTM && ENABLE_HL7

	if samePort {
		log.Println("📡 ASTM and HL7 sharing port:", ASTM_COM_PORT)
		StartCombinedListener()
	} else {
		// Start HL7 listener in separate goroutine if enabled and different port
		if ENABLE_HL7 && HL7_COM_PORT != "" {
			go StartHL7Listener()
		}

		// Start ASTM listener in main goroutine if enabled
		if ENABLE_ASTM && ASTM_COM_PORT != "" {
			StartASTMListener()
		} else {
			// If ASTM disabled, keep app running
			select {}
		}
	}
}

// ============================================================================
// COMBINED PROTOCOL HANDLER (Same Port)
// ============================================================================

func StartCombinedListener() {
	mode := &serial.Mode{
		BaudRate: ASTM_BAUD_RATE,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(ASTM_COM_PORT, mode)
	if err != nil {
		log.Fatal("❌ Failed to open serial port:", err)
	}
	defer port.Close()

	log.Println("✅ Combined Listener active on", ASTM_COM_PORT)

	readBuffer := make([]byte, 4096)
	var astmFrameBuffer bytes.Buffer
	var hl7MessageBuffer bytes.Buffer
	inHL7Message := false

	for {
		n, err := port.Read(readBuffer)
		if err != nil {
			log.Println("Read error:", err)
			continue
		}

		if DEBUG_MODE && n > 0 {
			log.Printf("[DEBUG] Received %d bytes: %s\n", n, hex.EncodeToString(readBuffer[:n]))
		}

		for _, b := range readBuffer[:n] {
			// Check for HL7 MLLP start
			if b == VT {
				inHL7Message = true
				hl7MessageBuffer.Reset()
				log.Println("➡️ [HL7] Message Start")
				continue
			}

			// If in HL7 message
			if inHL7Message {
				if b == FS {
					inHL7Message = false
					log.Println("⬅️ [HL7] Message End")
					processHL7Message(hl7MessageBuffer.String(), port)
					continue
				}
				hl7MessageBuffer.WriteByte(b)
				continue
			}

			// Otherwise, process as ASTM
			switch b {
			case ENQ:
				log.Println("➡️ [ASTM] ENQ received")
				port.Write([]byte{ACK})

			case STX:
				astmFrameBuffer.Reset()

			case ETX, ETB:
				port.Write([]byte{ACK})
				processASTMFrame(astmFrameBuffer.String())

			case EOT:
				log.Println("⬅️ [ASTM] EOT received")

			default:
				astmFrameBuffer.WriteByte(b)
			}
		}
	}
}

// ============================================================================
// ASTM PROTOCOL HANDLER
// ============================================================================

func StartASTMListener() {
	mode := &serial.Mode{
		BaudRate: ASTM_BAUD_RATE,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(ASTM_COM_PORT, mode)
	if err != nil {
		log.Println("⚠️ ASTM port not available:", err)
		log.Println("ASTM listener disabled")
		select {} // Block forever
	}
	defer port.Close()

	log.Println("✅ ASTM Listener active on", ASTM_COM_PORT)

	readBuffer := make([]byte, 256)
	var frameBuffer bytes.Buffer

	for {
		n, err := port.Read(readBuffer)
		if err != nil {
			log.Println("ASTM Read error:", err)
			continue
		}

		for _, b := range readBuffer[:n] {
			switch b {
			case ENQ:
				log.Println("➡️ [ASTM] ENQ received")
				port.Write([]byte{ACK})

			case STX:
				frameBuffer.Reset()

			case ETX, ETB:
				port.Write([]byte{ACK})
				processASTMFrame(frameBuffer.String())

			case EOT:
				log.Println("⬅️ [ASTM] EOT received")

			default:
				frameBuffer.WriteByte(b)
			}
		}
	}
}

func processASTMFrame(raw string) {
	log.Println("📦 [ASTM] Frame received")

	results := parseASTM(raw)
	if len(results) == 0 {
		log.Println("⚠️ No ASTM results found")
		return
	}

	sendASTMToServer(results)
}

func parseASTM(data string) []map[string]interface{} {
	lines := bytes.Split([]byte(data), []byte{CR})
	results := []map[string]interface{}{}

	var patientID, sampleID string

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		fields := bytes.Split(line, []byte("|"))
		recordType := string(fields[0])

		switch recordType {
		case "P":
			patientID = string(getASTMField(fields, 2))

		case "O":
			sampleID = string(getASTMField(fields, 2))

		case "R":
			result := map[string]interface{}{
				"protocol":   "ASTM",
				"patient_id": patientID,
				"sample_id":  sampleID,
				"test_code":  string(getASTMField(fields, 2)),
				"value":      string(getASTMField(fields, 3)),
				"units":      string(getASTMField(fields, 4)),
				"flags":      string(getASTMField(fields, 6)),
				"timestamp":  time.Now().Format(time.RFC3339),
			}
			results = append(results, result)
		}
	}
	return results
}

func getASTMField(fields [][]byte, index int) []byte {
	if index >= len(fields) {
		return []byte("")
	}
	return fields[index]
}

func sendASTMToServer(results []map[string]interface{}) {
	if DEBUG_MODE {
		fmt.Println("[DEBUG] ASTM Results:", results)
	}

	payload, err := json.Marshal(results)
	if err != nil {
		log.Println("❌ JSON error:", err)
		return
	}

	req, err := http.NewRequest("POST", ASTM_SERVER_ENDPOINT, bytes.NewBuffer(payload))
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

	log.Println("✅ [ASTM] Results forwarded:", resp.Status)
}

// ============================================================================
// HL7 PROTOCOL HANDLER
// ============================================================================

func StartHL7Listener() {
	mode := &serial.Mode{
		BaudRate: HL7_BAUD_RATE,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(HL7_COM_PORT, mode)
	if err != nil {
		log.Println("⚠️ HL7 port not available:", err)
		log.Println("HL7 listener disabled")
		return
	}
	defer port.Close()

	log.Println("✅ HL7 Listener active on", HL7_COM_PORT)

	readBuffer := make([]byte, 4096)
	var messageBuffer bytes.Buffer
	inMessage := false

	for {
		n, err := port.Read(readBuffer)
		if err != nil {
			log.Println("❌ HL7 Read error:", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if DEBUG_MODE && n > 0 {
			log.Printf("[DEBUG] HL7 Received %d bytes: %s\n", n, hex.EncodeToString(readBuffer[:n]))
		}

		for _, b := range readBuffer[:n] {
			if DEBUG_MODE {
				log.Printf("[DEBUG] Processing byte: 0x%02X (%c)\n", b, printable(b))
			}

			switch b {
			case VT:
				inMessage = true
				messageBuffer.Reset()
				log.Println("➡️ [HL7] Message Start (VT received)")

			case FS:
				if inMessage {
					inMessage = false
					log.Println("⬅️ [HL7] Message End (FS received)")
					processHL7Message(messageBuffer.String(), port)
				} else {
					log.Println("⚠️ [HL7] FS received but not in message")
				}

			case CR:
				if inMessage {
					messageBuffer.WriteByte(b)
					if DEBUG_MODE {
						log.Println("[DEBUG] CR added to message buffer")
					}
				}

			case LF:
				if inMessage {
					// Some systems send CRLF
					if DEBUG_MODE {
						log.Println("[DEBUG] LF received (ignored)")
					}
				}

			default:
				if inMessage {
					messageBuffer.WriteByte(b)
				}
			}
		}
	}
}

func processHL7Message(message string, port serial.Port) {
	log.Println("📦 [HL7] Message Received")
	log.Println("=" + strings.Repeat("=", 70))

	if DEBUG_MODE {
		log.Println("[DEBUG] Raw message:")
		log.Println(message)
		log.Println("=" + strings.Repeat("=", 70))
	}

	results := parseHL7(message)

	// Send ACK back to instrument
	ack := generateHL7ACK(message)

	if ack == "" {
		log.Println("⚠️ [HL7] Could not generate ACK - invalid message format")
	} else {
		ackBytes := []byte{VT}
		ackBytes = append(ackBytes, []byte(ack)...)
		ackBytes = append(ackBytes, FS, CR)

		_, err := port.Write(ackBytes)
		if err != nil {
			log.Println("❌ [HL7] Error sending ACK:", err)
		} else {
			log.Println("✅ [HL7] ACK sent")
			if DEBUG_MODE {
				log.Printf("[DEBUG] ACK: %s\n", ack)
			}
		}
	}

	if len(results) > 0 {
		sendHL7ToServer(results)
	} else {
		log.Println("⚠️ No results found in HL7 message")
		if DEBUG_MODE {
			log.Println("[DEBUG] Check if OBX segments are present")
		}
	}
}

func parseHL7(message string) []map[string]interface{} {
	message = strings.ReplaceAll(message, "\r\n", "\r")
	segments := strings.Split(message, string(CR))

	results := []map[string]interface{}{}

	var patientID, patientName, accessionNumber string
	var messageControlID string

	if DEBUG_MODE {
		log.Printf("[DEBUG] Found %d segments\n", len(segments))
	}

	for i, segment := range segments {
		segment = strings.TrimSpace(segment)
		if len(segment) == 0 {
			continue
		}

		if DEBUG_MODE {
			log.Printf("[DEBUG] Segment %d: %s\n", i, segment)
		}

		fields := strings.Split(segment, "|")
		if len(fields) == 0 {
			continue
		}

		segmentType := fields[0]

		switch segmentType {
		case "MSH":
			messageControlID = getHL7Field(fields, 9)
			if DEBUG_MODE {
				log.Printf("[DEBUG] MSH - Message Control ID: %s\n", messageControlID)
			}

		case "PID":
			patientID = getHL7Field(fields, 3)
			patientName = getHL7Field(fields, 5)
			if DEBUG_MODE {
				log.Printf("[DEBUG] PID - Patient ID: %s, Name: %s\n", patientID, patientName)
			}

		case "OBR":
			accessionNumber = getHL7Field(fields, 2)
			if DEBUG_MODE {
				log.Printf("[DEBUG] OBR - Accession: %s\n", accessionNumber)
			}

		case "OBX":
			if DEBUG_MODE {
				log.Printf("[DEBUG] OBX found with %d fields\n", len(fields))
			}

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

			if DEBUG_MODE {
				log.Printf("[DEBUG] Parsed OBX: Test=%s, Value=%s\n",
					result["test_code"], result["value"])
			}
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
			log.Printf("[DEBUG] Invalid MSH segment, only %d fields\n", len(mshFields))
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

func sendHL7ToServer(results []map[string]interface{}) {
	if DEBUG_MODE {
		log.Println("[DEBUG] Sending HL7 results to server:")
		for i, r := range results {
			log.Printf("[DEBUG] Result %d: %+v\n", i+1, r)
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

	log.Println("✅ [HL7] Results forwarded:", resp.Status)
}

func printable(b byte) rune {
	if b >= 32 && b <= 126 {
		return rune(b)
	}
	return '.'
}
