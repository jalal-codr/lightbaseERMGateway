package hl7

import (
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"
)

// ParseMessage parses an HL7 message and extracts lab results
func ParseMessage(message string) []map[string]interface{} {
	message = strings.ReplaceAll(message, "\r\n", "\r")
	segments := strings.Split(message, string(config.CR))

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
			messageControlID = getField(fields, 9)
		case "PID":
			patientID = getField(fields, 3)
			patientName = getField(fields, 5)
		case "OBR":
			accessionNumber = getField(fields, 2)
		case "OBX":
			result := map[string]interface{}{
				"patient_id":       patientID,
				"patient_name":     patientName,
				"accession_number": accessionNumber,
				"message_id":       messageControlID,
				"observation_id":   getField(fields, 1),
				"value_type":       getField(fields, 2),
				"test_code":        parseComponent(getField(fields, 3), 0),
				"test_name":        parseComponent(getField(fields, 3), 1),
				"value":            getField(fields, 5),
				"units":            getField(fields, 6),
				"reference_range":  getField(fields, 7),
				"abnormal_flags":   getField(fields, 8),
				"result_status":    getField(fields, 11),
				"timestamp":        parseDateTime(getField(fields, 14)),
			}
			results = append(results, result)
		}
	}

	return results
}

func getField(fields []string, index int) string {
	if index >= len(fields) {
		return ""
	}
	return strings.TrimSpace(fields[index])
}

func parseComponent(field string, componentIndex int) string {
	components := strings.Split(field, "^")
	if componentIndex >= len(components) {
		return ""
	}
	return strings.TrimSpace(components[componentIndex])
}

func parseDateTime(hl7DateTime string) string {
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
