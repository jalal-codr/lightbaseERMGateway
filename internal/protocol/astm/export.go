package astm

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"
	"lightbaseEMRProxy/internal/protocol/hl7"
	"lightbaseEMRProxy/types"
)

// LabReport represents the final JSON output structure
type LabReport struct {
	MessageID  string       `json:"message_id"`
	ReceivedAt string       `json:"received_at"`
	Instrument Instrument   `json:"instrument"`
	Patient    PatientInfo  `json:"patient"`
	Order      OrderInfo    `json:"order"`
	Results    []ResultItem `json:"results"`
}

type Instrument struct {
	ID              string `json:"id"`
	SoftwareVersion string `json:"software_version"`
	SerialNumber    string `json:"serial_number"`
}

type PatientInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Sex  string `json:"sex"`
}

type OrderInfo struct {
	SpecimenID  string `json:"specimen_id"`
	Priority    string `json:"priority"`
	CollectedAt string `json:"collected_at"`
	ReportType  string `json:"report_type"`
}

type ResultItem struct {
	ObservationID  string `json:"observation_id"`
	TestCode       string `json:"test_code"`
	TestName       string `json:"test_name"`
	Value          string `json:"value"`
	Units          string `json:"units"`
	ReferenceRange string `json:"reference_range"`
	AbnormalFlags  string `json:"abnormal_flags"`
	Status         string `json:"status"`
	TestedAt       string `json:"tested_at"`
	InstrumentID   string `json:"instrument_id"`
	IsAbnormal     bool   `json:"is_abnormal"`
	Remark         string `json:"remark"`
}

// ParseASTMMessage parses an ASTM E1394 message and returns results + JSON report
func ParseASTMMessage(message string) ([]map[string]interface{}, string, error) {
	// Normalize line endings
	message = strings.ReplaceAll(message, "\r\n", "\r")
	message = strings.ReplaceAll(message, "\n", "\r")

	// Strip STX (0x02) and ETX (0x03) control characters
	message = strings.Map(func(r rune) rune {
		if r == 0x02 || r == 0x03 {
			return -1
		}
		return r
	}, message)

	segments := strings.Split(message, "\r")

	results := []map[string]interface{}{}

	// Context variables populated as we walk records
	var (
		messageID       string
		instrumentID    string
		softwareVersion string
		serialNumber    string
		patientID       string
		patientName     string
		patientSex      string
		specimenID      string
		priority        string
		collectedAt     string
		reportType      string
	)

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		fields := strings.Split(segment, "|")
		if len(fields) == 0 {
			continue
		}

		recordType := fields[0]

		switch recordType {

		// H - Header Record
		// H|\^&|||SenderID^SWVersion^SerialNo||||||||PR|1394-97|timestamp
		case "H":
			senderComponents := strings.Split(getASTMField(fields, 4), "^")
			instrumentID = safeComponent(senderComponents, 0)
			softwareVersion = safeComponent(senderComponents, 1)
			serialNumber = safeComponent(senderComponents, 2)
			messageID = getASTMField(fields, 12) // Processing ID or version
			_ = messageID

		// P - Patient Record
		// P|seq|patientID|||name||dob|sex|...
		case "P":
			patientID = getASTMField(fields, 2)
			patientName = getASTMField(fields, 5)
			sexRaw := parseASTMComponent(getASTMField(fields, 7), 0)
			patientSex = decodeSex(sexRaw)

		// O - Order Record
		// O|seq|specimenID||testID|priority|collectedAt|...|reportType
		case "O":
			specimenID = parseASTMComponent(getASTMField(fields, 2), 0)
			priority = decodePriority(getASTMField(fields, 5))
			collectedAt = parseASTMDateTime(getASTMField(fields, 7))
			reportType = decodeReportType(getASTMField(fields, 25))

		// R - Result Record
		// R|seq|testID^testName^flag|value^^unit_arrow|units|low^high||flags|...|testedAt|...|instrumentID
		case "R":
			testField := getASTMField(fields, 2)
			testComponents := strings.Split(testField, "^")

			testCode := safeComponent(testComponents, 0)
			testName := safeComponent(testComponents, 1)
			resultFlag := safeComponent(testComponents, 2) // e.g. "F"

			// Value field may contain embedded arrow characters (↑ ↓)
			rawValue := getASTMField(fields, 3)
			valueComponents := strings.Split(rawValue, "^")
			value := cleanValue(safeComponent(valueComponents, 0))

			units := getASTMField(fields, 4)

			// Reference range: "low^high"
			refRaw := getASTMField(fields, 5)
			refRange := formatReferenceRange(refRaw)

			abnormalFlags := getASTMField(fields, 6)
			testedAt := parseASTMDateTime(getASTMField(fields, 12))
			resultInstrument := parseASTMComponent(getASTMField(fields, 14), 0)

			if resultInstrument == "" {
				resultInstrument = instrumentID
			}

			isAbnormal := isASTMResultAbnormal(abnormalFlags, value, refRaw)
			remark := buildASTMRemark(abnormalFlags, value, refRaw, isAbnormal)

			result := map[string]interface{}{
				"observation_id":  getASTMField(fields, 1),
				"test_code":       testCode,
				"test_name":       testName,
				"result_flag":     resultFlag,
				"value":           value,
				"units":           units,
				"reference_range": refRange,
				"abnormal_flags":  abnormalFlags,
				"result_status":   resultFlag,
				"tested_at":       testedAt,
				"instrument_id":   resultInstrument,
				"is_abnormal":     isAbnormal,
				"remark":          remark,
			}
			results = append(results, result)
		}
	}

	// Build the report
	report := LabReport{
		MessageID:  messageID,
		ReceivedAt: time.Now().Format(time.RFC3339),
		Instrument: Instrument{
			ID:              instrumentID,
			SoftwareVersion: softwareVersion,
			SerialNumber:    serialNumber,
		},
		Patient: PatientInfo{
			ID:   patientID,
			Name: patientName,
			Sex:  patientSex,
		},
		Order: OrderInfo{
			SpecimenID:  specimenID,
			Priority:    priority,
			CollectedAt: collectedAt,
			ReportType:  reportType,
		},
	}

	for _, r := range results {
		report.Results = append(report.Results, ResultItem{
			ObservationID:  r["observation_id"].(string),
			TestCode:       r["test_code"].(string),
			TestName:       r["test_name"].(string),
			Value:          r["value"].(string),
			Units:          r["units"].(string),
			ReferenceRange: r["reference_range"].(string),
			AbnormalFlags:  r["abnormal_flags"].(string),
			Status:         r["result_status"].(string),
			TestedAt:       r["tested_at"].(string),
			InstrumentID:   r["instrument_id"].(string),
			IsAbnormal:     r["is_abnormal"].(bool),
			Remark:         r["remark"].(string),
		})
	}

	// Also build types.HL7Message for external saver (reusing your existing type)
	now := time.Now().Format(time.RFC3339)
	payload := types.HL7Message{
		Source:     "astm_bridge",
		MessageID:  messageID,
		ReceivedAt: now,
		CreatedAt:  now,
		Patient: types.HL7Patient{
			ID:   patientID,
			Name: patientName,
		},
		Order: types.HL7Order{
			AccessionNumber: specimenID,
		},
	}
	for _, r := range report.Results {
		payload.Results = append(payload.Results, types.HL7Result{
			ObservationID:  r.ObservationID,
			TestCode:       r.TestCode,
			TestName:       r.TestName,
			Value:          r.Value,
			Units:          r.Units,
			ReferenceRange: r.ReferenceRange,
			AbnormalFlags:  r.AbnormalFlags,
			Status:         r.Status,
			Timestamp:      r.TestedAt,
		})
	}

	go func() {
		if err := hl7.SendToExternalSaver(payload, config.ExternalSaverURL); err != nil {
			log.Printf("ASTM forward failed [%s]: %v", messageID, err)
		}
	}()

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return results, "", fmt.Errorf("failed to marshal report: %w", err)
	}

	return results, string(jsonBytes), nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getASTMField(fields []string, index int) string {
	if index >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[index])
}

func parseASTMComponent(field string, index int) string {
	parts := strings.Split(field, "^")
	return safeComponent(parts, index)
}

func safeComponent(parts []string, index int) string {
	if index >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[index])
}

// cleanValue strips embedded Unicode arrows (↑ ↓) from value strings
func cleanValue(v string) string {
	v = strings.ReplaceAll(v, "↑", "")
	v = strings.ReplaceAll(v, "↓", "")
	return strings.TrimSpace(v)
}

// formatReferenceRange converts "0.003^4.000" → "0.003 - 4.000"
func formatReferenceRange(raw string) string {
	parts := strings.Split(raw, "^")
	if len(parts) == 2 {
		return fmt.Sprintf("%s - %s", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}
	return raw
}

func parseASTMDateTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 14 {
		t, err := time.Parse("20060102150405", raw[:14])
		if err == nil {
			return t.Format(time.RFC3339)
		}
	}
	if len(raw) >= 8 {
		t, err := time.Parse("20060102", raw[:8])
		if err == nil {
			return t.Format(time.RFC3339)
		}
	}
	return ""
}

func isASTMResultAbnormal(flags, value, refRaw string) bool {
	f := strings.ToUpper(strings.TrimSpace(flags))
	if f == "H" || f == "L" || f == "HH" || f == "LL" || f == "A" {
		return true
	}
	// Check embedded arrows in original value
	if strings.Contains(value, "↑") || strings.Contains(value, "↓") {
		return true
	}
	// Numeric comparison against reference range
	parts := strings.Split(refRaw, "^")
	if len(parts) == 2 {
		var low, high, val float64
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &low); err != nil {
			return false
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &high); err != nil {
			return false
		}
		if _, err := fmt.Sscanf(cleanValue(value), "%f", &val); err != nil {
			return false
		}
		return val < low || val > high
	}
	return false
}

func buildASTMRemark(flags, value, refRaw string, isAbnormal bool) string {
	f := strings.ToUpper(strings.TrimSpace(flags))
	switch f {
	case "H":
		return "High"
	case "HH":
		return "Critically High"
	case "L":
		return "Low"
	case "LL":
		return "Critically Low"
	case "A":
		return "Abnormal"
	}
	if strings.Contains(value, "↑") {
		return "High"
	}
	if strings.Contains(value, "↓") {
		return "Low"
	}
	if isAbnormal {
		// Determine direction from numeric comparison
		parts := strings.Split(refRaw, "^")
		if len(parts) == 2 {
			var high, val float64
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &high)
			fmt.Sscanf(cleanValue(value), "%f", &val)
			if val > high {
				return "High"
			}
			return "Low"
		}
		return "Out of Range"
	}
	return "Normal"
}

func decodeSex(raw string) string {
	switch strings.ToUpper(raw) {
	case "M":
		return "Male"
	case "F":
		return "Female"
	case "0", "U", "":
		return "Unknown"
	default:
		return raw
	}
}

func decodePriority(raw string) string {
	switch strings.ToUpper(raw) {
	case "R":
		return "Routine"
	case "S":
		return "STAT"
	case "A":
		return "ASAP"
	default:
		return raw
	}
}

func decodeReportType(raw string) string {
	switch strings.ToUpper(raw) {
	case "F":
		return "Final"
	case "P":
		return "Preliminary"
	case "C":
		return "Corrected"
	default:
		return raw
	}
}
