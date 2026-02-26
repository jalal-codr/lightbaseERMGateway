package astm

import (
	"log"
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"
	"lightbaseEMRProxy/internal/logger"
	"lightbaseEMRProxy/internal/protocol/hl7"
	"lightbaseEMRProxy/types"
)

func ProcessMessage(message string) {
	log.Println("\n📦 [ASTM] MESSAGE RECEIVED")
	if config.DebugMode {
		log.Println("Raw ASTM:\n", message)
		log.Println(strings.Repeat("-", 60))
	}

	records := strings.Split(message, string(config.CR))
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
			patientID = getField(fields, 3)
			patientName = getField(fields, 5)
			log.Printf("[ASTM] Patient: ID=%s  Name=%s\n", patientID, patientName)
		case "O":
			orderID = getField(fields, 2)
			log.Printf("[ASTM] Order: ID=%s\n", orderID)
		case "R":
			result := map[string]interface{}{
				"test_code":       parseComponent(getField(fields, 2), 3),
				"test_name":       parseComponent(getField(fields, 2), 4),
				"value":           getField(fields, 3),
				"units":           getField(fields, 4),
				"reference_range": getField(fields, 5),
				"abnormal_flags":  getField(fields, 6),
				"result_status":   getField(fields, 8),
				"timestamp":       parseDateTime(getField(fields, 12)),
			}
			results = append(results, result)
		case "L":
			log.Println("[ASTM] Terminator record")
		}
	}

	if config.LogToTerminal && len(results) > 0 {
		logger.LogResults(results)
	} else if len(results) == 0 {
		log.Println("⚠️  [ASTM] No R (result) records found in message")
		return
	}

	// Build typed payload
	now := time.Now().Format(time.RFC3339)
	payload := types.HL7Message{
		Source:     "astm_bridge",
		MessageID:  orderID, // use orderID as the message identifier
		ReceivedAt: now,
		CreatedAt:  now,
		Patient: types.HL7Patient{
			ID:   patientID,
			Name: patientName,
		},
		Order: types.HL7Order{
			AccessionNumber: orderID,
		},
	}

	for _, r := range results {
		payload.Results = append(payload.Results, types.HL7Result{
			ObservationID:  "", // ASTM R records have no direct observation ID
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

	// Send async, non-blocking
	go func() {
		if err := hl7.SendToExternalSaver(payload, config.ExternalSaverURL); err != nil {
			log.Printf("❌ [ASTM] Forward failed [%s]: %v", orderID, err)
		} else {
			log.Printf("✅ [ASTM] Forwarded successfully [%s]", orderID)
		}
	}()
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

func parseDateTime(dateTime string) string {
	dateTime = strings.TrimSpace(dateTime)
	if len(dateTime) < 8 {
		return time.Now().Format(time.RFC3339)
	}

	layout := "20060102150405"
	if len(dateTime) >= 14 {
		t, err := time.Parse(layout, dateTime[:14])
		if err == nil {
			return t.Format(time.RFC3339)
		}
	}

	layout = "20060102"
	t, err := time.Parse(layout, dateTime[:8])
	if err == nil {
		return t.Format(time.RFC3339)
	}

	return time.Now().Format(time.RFC3339)
}
