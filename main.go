package main

import (
	"bytes"
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
	HL7_COM_PORT        = "COM2"
	HL7_BAUD_RATE       = 9600
	HL7_SERVER_ENDPOINT = "https://your-server.com/api/lis/hl7-results"
)

/*
ENTRY POINT - Runs both ASTM and HL7 listeners
*/
func main() {
	log.Println("🚀 Starting Multi-Protocol LIS Application")
	log.Println(strings.Repeat("=", 50))

	// Start HL7 listener in separate goroutine
	go StartHL7Listener()

	// Start ASTM listener in main goroutine
	StartASTMListener()
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
		log.Fatal("❌ Failed to open ASTM serial port:", err)
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
		log.Fatal("❌ Failed to open HL7 serial port:", err)
	}
	defer port.Close()

	log.Println("✅ HL7 Listener active on", HL7_COM_PORT)

	readBuffer := make([]byte, 4096)
	var messageBuffer bytes.Buffer
	inMessage := false

	for {
		n, err := port.Read(readBuffer)
		if err != nil {
			log.Println("HL7 Read error:", err)
			continue
		}

		for _, b := range readBuffer[:n] {
			switch b {
			case VT:
				inMessage = true
				messageBuffer.Reset()
				log.Println("➡️ [HL7] Message Start")

			case FS:
				if inMessage {
					inMessage = false
					processHL7Message(messageBuffer.String(), port)
				}

			case CR:
				if inMessage {
					messageBuffer.WriteByte(b)
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

	results := parseHL7(message)

	// Send ACK back to instrument
	ack := generateHL7ACK(message)
	ackBytes := []byte{VT}
	ackBytes = append(ackBytes, []byte(ack)...)
	ackBytes = append(ackBytes, FS, CR)

	port.Write(ackBytes)
	log.Println("✅ [HL7] ACK sent")

	if len(results) > 0 {
		sendHL7ToServer(results)
	} else {
		log.Println("⚠️ No results found in HL7 message")
	}
}

func parseHL7(message string) []map[string]interface{} {
	segments := strings.Split(message, string(CR))
	results := []map[string]interface{}{}

	var patientID, patientName, accessionNumber string
	var messageControlID string

	for _, segment := range segments {
		if len(segment) == 0 {
			continue
		}

		fields := strings.Split(segment, "|")
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
	return fields[index]
}

func parseHL7Component(field string, componentIndex int) string {
	components := strings.Split(field, "^")
	if componentIndex >= len(components) {
		return ""
	}
	return components[componentIndex]
}

func parseHL7DateTime(hl7DateTime string) string {
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
	segments := strings.Split(originalMessage, string(CR))
	var mshFields []string

	for _, segment := range segments {
		if strings.HasPrefix(segment, "MSH") {
			mshFields = strings.Split(segment, "|")
			break
		}
	}

	if len(mshFields) == 0 {
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

	ack += fmt.Sprintf("MSA%sAA%s%s%s",
		fieldSeparator,
		fieldSeparator,
		messageControlID,
		fieldSeparator,
	)

	return ack
}

func sendHL7ToServer(results []map[string]interface{}) {
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
