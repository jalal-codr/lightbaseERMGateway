package hl7

import (
	"log"
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"
	"lightbaseEMRProxy/types"
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
				"observation_id":  getField(fields, 1),
				"test_code":       parseComponent(getField(fields, 3), 0),
				"test_name":       parseComponent(getField(fields, 3), 1),
				"value":           getField(fields, 5),
				"units":           getField(fields, 6),
				"reference_range": getField(fields, 7),
				"abnormal_flags":  getField(fields, 8),
				"result_status":   getField(fields, 11),
				"timestamp":       parseDateTime(getField(fields, 14)),
			}
			results = append(results, result)
		}
	}

	// Build HL7Message (matches server's expected type exactly)
	now := time.Now().Format(time.RFC3339)
	payload := types.HL7Message{
		Source:     "hl7_bridge",
		MessageID:  messageControlID,
		ReceivedAt: now,
		CreatedAt:  now,
		Patient: types.HL7Patient{
			ID:   patientID,
			Name: patientName,
		},
		Order: types.HL7Order{
			AccessionNumber: accessionNumber,
		},
	}

	for _, r := range results {
		payload.Results = append(payload.Results, types.HL7Result{
			ObservationID:  r["observation_id"].(string),
			TestCode:       r["test_code"].(string),
			TestName:       r["test_name"].(string),
			Value:          r["value"].(string),
			Units:          r["units"].(string),
			ReferenceRange: r["reference_range"].(string),
			AbnormalFlags:  r["abnormal_flags"].(string),
			Status:         r["result_status"].(string),
			Timestamp:      r["timestamp"].(string),
		})
	}

	go func() {
		if err := SendToExternalSaver(payload, config.ExternalSaverURL+"/hl7/receive"); err != nil {
			log.Printf("HL7 forward failed [%s]: %v", messageControlID, err)
		}
	}()

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
