package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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
	// Serial Port - TRY THESE IN ORDER: COM1, COM2, COM3, COM4
	// Or on Linux: /dev/ttyUSB0, /dev/ttyS0
	COM_PORT = "COM1" // <-- CHANGE THIS TO YOUR PORT

	BAUD_RATE = 9600

	// Server endpoints
	ASTM_SERVER_ENDPOINT = "https://your-server.com/api/lis/astm-results"
	HL7_SERVER_ENDPOINT  = "https://your-server.com/api/lis/hl7-results"

	// Protocol enables
	ENABLE_ASTM = true
	ENABLE_HL7  = true

	// Debug mode - ALWAYS TRUE for troubleshooting
	DEBUG_MODE = true

	// Log all raw bytes to file
	LOG_RAW_BYTES = true
)

var rawDataFile *os.File

/*
ENTRY POINT
*/
func main() {
	log.Println("🚀 Starting LIS Application - DIAGNOSTIC MODE")
	log.Println(strings.Repeat("=", 60))

	// List available ports
	listAvailablePorts()

	// Open raw data log file
	if LOG_RAW_BYTES {
		var err error
		rawDataFile, err = os.Create("raw_serial_data.log")
		if err != nil {
			log.Println("⚠️ Could not create log file:", err)
		} else {
			defer rawDataFile.Close()
			log.Println("📝 Logging raw data to: raw_serial_data.log")
		}
	}

	// Start combined listener
	StartCombinedListener()
}

// List all available serial ports
func listAvailablePorts() {
	log.Println("\n🔍 Searching for available serial ports...")

	ports, err := serial.GetPortsList()
	if err != nil {
		log.Println("⚠️ Error listing ports:", err)
		return
	}

	if len(ports) == 0 {
		log.Println("❌ No serial ports found!")
		log.Println("   Please check:")
		log.Println("   1. Device is connected via USB/Serial")
		log.Println("   2. Drivers are installed")
		log.Println("   3. Cable is properly connected")
		return
	}

	log.Println("✅ Found", len(ports), "port(s):")
	for i, port := range ports {
		log.Printf("   %d. %s\n", i+1, port)
	}
	log.Println("\n💡 Update COM_PORT constant to match your device port")
	log.Println(strings.Repeat("=", 60) + "\n")
}

// Combined protocol handler
func StartCombinedListener() {
	mode := &serial.Mode{
		BaudRate: BAUD_RATE,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	log.Printf("📡 Opening port: %s at %d baud\n", COM_PORT, BAUD_RATE)
	port, err := serial.Open(COM_PORT, mode)
	if err != nil {
		log.Println("❌ Failed to open port:", err)
		log.Println("\n💡 TROUBLESHOOTING:")
		log.Println("   - Check the port name is correct (see list above)")
		log.Println("   - Make sure no other program is using this port")
		log.Println("   - Try unplugging and replugging the device")
		log.Println("   - On Linux/Mac, check permissions: sudo chmod 666 /dev/ttyUSB0")
		return
	}
	defer port.Close()

	log.Println("✅ Port opened successfully!")
	log.Println("⏳ Waiting for data...")
	log.Println(strings.Repeat("=", 60))

	readBuffer := make([]byte, 4096)
	var astmFrameBuffer bytes.Buffer
	var hl7MessageBuffer bytes.Buffer
	inHL7Message := false
	byteCount := 0

	for {
		n, err := port.Read(readBuffer)
		if err != nil {
			log.Println("❌ Read error:", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if n > 0 {
			byteCount += n

			// Log that we received data
			log.Printf("\n🔔 RECEIVED %d bytes (total: %d bytes)\n", n, byteCount)
			log.Println(strings.Repeat("-", 60))

			// Show hex dump
			hexStr := hex.EncodeToString(readBuffer[:n])
			log.Printf("HEX: %s\n", hexStr)

			// Show ASCII (printable characters)
			asciiStr := ""
			for _, b := range readBuffer[:n] {
				if b >= 32 && b <= 126 {
					asciiStr += string(b)
				} else {
					asciiStr += fmt.Sprintf("[0x%02X]", b)
				}
			}
			log.Printf("ASCII: %s\n", asciiStr)
			log.Println(strings.Repeat("-", 60))

			// Log to file
			if rawDataFile != nil {
				timestamp := time.Now().Format("2006-01-02 15:04:05.000")
				rawDataFile.WriteString(fmt.Sprintf("\n[%s] Received %d bytes\n", timestamp, n))
				rawDataFile.WriteString(fmt.Sprintf("HEX: %s\n", hexStr))
				rawDataFile.WriteString(fmt.Sprintf("ASCII: %s\n\n", asciiStr))
			}
		}

		// Process bytes
		for i, b := range readBuffer[:n] {
			if DEBUG_MODE {
				log.Printf("Byte %d: 0x%02X (%s)\n", i, b, byteDescription(b))
			}

			// Check for HL7 MLLP start
			if b == VT {
				inHL7Message = true
				hl7MessageBuffer.Reset()
				log.Println("➡️ [HL7] Message Start (VT detected)")
				continue
			}

			// If in HL7 message
			if inHL7Message {
				if b == FS {
					inHL7Message = false
					log.Println("⬅️ [HL7] Message End (FS detected)")
					processHL7Message(hl7MessageBuffer.String(), port)
					continue
				}
				hl7MessageBuffer.WriteByte(b)
				continue
			}

			// Otherwise, process as ASTM
			switch b {
			case ENQ:
				log.Println("➡️ [ASTM] ENQ received - Sending ACK")
				port.Write([]byte{ACK})

			case STX:
				log.Println("➡️ [ASTM] STX - Frame Start")
				astmFrameBuffer.Reset()

			case ETX:
				log.Println("⬅️ [ASTM] ETX - Frame End")
				port.Write([]byte{ACK})
				processASTMFrame(astmFrameBuffer.String())

			case ETB:
				log.Println("⬅️ [ASTM] ETB - Frame End (more to come)")
				port.Write([]byte{ACK})
				processASTMFrame(astmFrameBuffer.String())

			case EOT:
				log.Println("⬅️ [ASTM] EOT - Transmission End")

			case CR:
				astmFrameBuffer.WriteByte(b)

			case LF:
				// Usually ignore LF in ASTM
				if DEBUG_MODE {
					log.Println("   [LF ignored]")
				}

			default:
				astmFrameBuffer.WriteByte(b)
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
	case ENQ:
		return "ENQ (ASTM Enquiry)"
	case ACK:
		return "ACK"
	case NAK:
		return "NAK"
	case STX:
		return "STX (ASTM Start)"
	case ETX:
		return "ETX (ASTM End)"
	case ETB:
		return "ETB (ASTM End Block)"
	case EOT:
		return "EOT (ASTM End Transmission)"
	case CR:
		return "CR (Carriage Return)"
	case LF:
		return "LF (Line Feed)"
	default:
		if b >= 32 && b <= 126 {
			return fmt.Sprintf("'%c'", b)
		}
		return "data"
	}
}

func processASTMFrame(raw string) {
	log.Println("\n📦 [ASTM] Processing Frame")
	log.Println("Content:", raw)
	log.Println(strings.Repeat("-", 60))

	results := parseASTM(raw)
	if len(results) == 0 {
		log.Println("⚠️ No ASTM results found")
		return
	}

	log.Printf("✅ Parsed %d ASTM result(s)\n", len(results))
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

		log.Printf("   Record Type: %s (fields: %d)\n", recordType, len(fields))

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
	log.Println("\n📤 Sending ASTM results to server...")
	for i, r := range results {
		log.Printf("   Result %d: %+v\n", i+1, r)
	}

	payload, _ := json.Marshal(results)
	req, _ := http.NewRequest("POST", ASTM_SERVER_ENDPOINT, bytes.NewBuffer(payload))
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

func processHL7Message(message string, port serial.Port) {
	log.Println("\n📦 [HL7] Processing Message")
	log.Println("Content:", message)
	log.Println(strings.Repeat("-", 60))

	results := parseHL7(message)

	// Send ACK
	ack := generateHL7ACK(message)
	if ack != "" {
		ackBytes := []byte{VT}
		ackBytes = append(ackBytes, []byte(ack)...)
		ackBytes = append(ackBytes, FS, CR)
		port.Write(ackBytes)
		log.Println("✅ [HL7] ACK sent")
	}

	if len(results) > 0 {
		log.Printf("✅ Parsed %d HL7 result(s)\n", len(results))
		sendHL7ToServer(results)
	}
}

func parseHL7(message string) []map[string]interface{} {
	message = strings.ReplaceAll(message, "\r\n", "\r")
	segments := strings.Split(message, string(CR))
	results := []map[string]interface{}{}
	var patientID, patientName, accessionNumber, messageControlID string

	log.Printf("   Found %d segments\n", len(segments))

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if len(segment) == 0 {
			continue
		}

		fields := strings.Split(segment, "|")
		segmentType := fields[0]
		log.Printf("   Segment: %s (fields: %d)\n", segmentType, len(fields))

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
				"test_code":        parseHL7Component(getHL7Field(fields, 3), 0),
				"test_name":        parseHL7Component(getHL7Field(fields, 3), 1),
				"value":            getHL7Field(fields, 5),
				"units":            getHL7Field(fields, 6),
				"timestamp":        time.Now().Format(time.RFC3339),
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

func generateHL7ACK(originalMessage string) string {
	originalMessage = strings.ReplaceAll(originalMessage, "\r\n", "\r")
	segments := strings.Split(originalMessage, string(CR))
	var mshFields []string

	for _, segment := range segments {
		if strings.HasPrefix(strings.TrimSpace(segment), "MSH") {
			mshFields = strings.Split(segment, "|")
			break
		}
	}

	if len(mshFields) < 10 {
		return ""
	}

	ack := fmt.Sprintf("MSH|%s|%s|%s|%s|%s|%s|ACK|%s|AL|",
		getHL7Field(mshFields, 1),
		getHL7Field(mshFields, 4),
		getHL7Field(mshFields, 5),
		getHL7Field(mshFields, 2),
		getHL7Field(mshFields, 3),
		time.Now().Format("20060102150405"),
		getHL7Field(mshFields, 9))
	ack += string(CR)
	ack += fmt.Sprintf("MSA|AA|%s|", getHL7Field(mshFields, 9))
	return ack
}

func sendHL7ToServer(results []map[string]interface{}) {
	log.Println("\n📤 Sending HL7 results to server...")
	for i, r := range results {
		log.Printf("   Result %d: %+v\n", i+1, r)
	}

	payload, _ := json.Marshal(results)
	req, _ := http.NewRequest("POST", HL7_SERVER_ENDPOINT, bytes.NewBuffer(payload))
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
