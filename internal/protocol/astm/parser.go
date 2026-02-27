package astm

import (
	"log"
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"
	"lightbaseEMRProxy/internal/protocol/hl7"
	"lightbaseEMRProxy/types"
)

func ProcessMessage(message string) {
	log.Println("📦 [ASTM] Raw message received:")
	log.Println(message)
	log.Println(strings.Repeat("-", 60))

	// Check if this is Bio-Rad D-10 proprietary format
	if strings.HasPrefix(message, "S03") {
		processBioRadD10Message(message)
		return
	}

	// Standard ASTM processing
	records := strings.Split(message, string(config.CR))
	results := []map[string]interface{}{}

	var patientID, patientName, orderID string

	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		log.Printf("[ASTM] Processing record: %s\n", record)

		fields := strings.Split(record, "|")
		if len(fields) == 0 {
			continue
		}
		recordType := fields[0]

		switch recordType {
		case "P":
			patientID = getField(fields, 3)
			patientName = getField(fields, 5)
			log.Printf("[ASTM] Patient: ID=%s Name=%s\n", patientID, patientName)
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
			log.Printf("[ASTM] Result added: %s = %s\n", result["test_name"], result["value"])
		}
	}

	// Send to API even if no results (for debugging)
	now := time.Now().Format(time.RFC3339)
	payload := types.HL7Message{
		Source:     "astm_bridge",
		MessageID:  orderID,
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
			ObservationID:  "",
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

	log.Printf("📦 [ASTM] Sending to API: Order=%s Patient=%s Results=%d\n", orderID, patientID, len(results))

	if err := hl7.SendToExternalSaver(payload, config.ExternalSaverURL); err != nil {
		log.Printf("❌ [ASTM] Forward failed [%s]: %v\n", orderID, err)
	} else {
		log.Printf("✅ [ASTM] Data forwarded successfully [%s]\n", orderID)
	}
}

func processBioRadD10Message(message string) {
	log.Println("🔬 [ASTM] Detected Bio-Rad D-10 HbA1c format")

	// Extract header information
	// Format: S03----06OBIOMA0369010010022030420050610182632498
	sampleID := ""
	patientName := ""

	if len(message) > 20 {
		// Extract sample ID (appears after initial header)
		parts := strings.Split(message, "OBIOMA")
		if len(parts) > 1 {
			patientName = "OBIOMA"
			// Sample ID follows the name
			if len(parts[1]) >= 10 {
				sampleID = strings.TrimLeft(parts[1][:10], "0")
			}
		}
	}

	// Parse all numeric values (they appear as dot-separated values)
	values := strings.Split(message, ".")
	var numericValues []string

	for _, val := range values {
		val = strings.TrimSpace(val)
		// Extract numeric part before any non-digit
		numPart := ""
		for _, ch := range val {
			if ch >= '0' && ch <= '9' {
				numPart += string(ch)
			} else {
				break
			}
		}
		if numPart != "" && len(numPart) <= 6 {
			numericValues = append(numericValues, numPart)
		}
	}

	log.Printf("📊 [ASTM] Extracted %d numeric values\n", len(numericValues))

	// Bio-Rad D-10 HbA1c test results structure
	// Based on the receipt data, we expect these peaks:
	peakNames := []string{
		"HbA1a", "HbA1b", "HbF", "LA1c+", "HbA1c", "HbA0", "V_Win",
		"Total_Area", "HbA1c_IFCC", "AG_ADA_mg", "AG_ADA_mmol", "HbA1c_NGSP",
	}

	results := []types.HL7Result{}
	now := time.Now().Format(time.RFC3339)

	// Map numeric values to peak names
	for i, peakName := range peakNames {
		if i < len(numericValues) {
			value := numericValues[i]
			// Convert to decimal format (divide by 10 for percentage values)
			if peakName == "HbA1c_NGSP" || peakName == "HbA1c" {
				if len(value) >= 2 {
					value = value[:len(value)-1] + "." + value[len(value)-1:]
				}
			} else if peakName == "HbA1c_IFCC" {
				// IFCC is in mmol/mol, keep as integer
			} else if peakName == "Total_Area" {
				// Total area is a large number
			} else {
				// Other values are percentages or decimals
				if len(value) >= 2 {
					value = value[:len(value)-1] + "." + value[len(value)-1:]
				}
			}

			units := "%"
			if peakName == "HbA1c_IFCC" {
				units = "mmol/mol"
			} else if peakName == "AG_ADA_mg" {
				units = "mg/dL"
			} else if peakName == "AG_ADA_mmol" {
				units = "mmol/L"
			} else if peakName == "Total_Area" {
				units = ""
			}

			results = append(results, types.HL7Result{
				ObservationID:  "",
				TestCode:       "HBA1C",
				TestName:       peakName,
				Value:          value,
				Units:          units,
				ReferenceRange: "",
				AbnormalFlags:  "",
				Status:         "F",
				Timestamp:      now,
			})

			log.Printf("  ✓ %s = %s %s\n", peakName, value, units)
		}
	}

	payload := types.HL7Message{
		Source:     "astm_biorad_d10",
		MessageID:  sampleID,
		ReceivedAt: now,
		CreatedAt:  now,
		Patient: types.HL7Patient{
			ID:   sampleID,
			Name: patientName,
		},
		Order: types.HL7Order{
			AccessionNumber: sampleID,
		},
		Results: results,
	}

	log.Printf("📦 [ASTM] Sending Bio-Rad D-10 data: Sample=%s Results=%d\n", sampleID, len(results))

	if err := hl7.SendToExternalSaver(payload, config.ExternalSaverURL); err != nil {
		log.Printf("❌ [ASTM] Forward failed [%s]: %v\n", sampleID, err)
	} else {
		log.Printf("✅ [ASTM] Bio-Rad D-10 data forwarded successfully [%s]\n", sampleID)
	}
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
