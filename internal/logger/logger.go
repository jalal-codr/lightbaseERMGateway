package logger

import (
	"encoding/json"
	"log"
	"strings"
)

// LogResults logs lab results to the terminal
func LogResults(results []map[string]interface{}) {
	log.Println("\n" + strings.Repeat("*", 60))
	log.Println("*** LAB RESULTS - TERMINAL OUTPUT ***")
	log.Println(strings.Repeat("*", 60))

	for i, result := range results {
		log.Printf("\n📋 Result #%d:\n", i+1)
		log.Println(strings.Repeat("-", 60))
		log.Println("👤 PATIENT INFORMATION:")
		log.Printf("   Patient ID:       %v\n", result["patient_id"])
		log.Printf("   Patient Name:     %v\n", result["patient_name"])
		if accNum, ok := result["accession_number"]; ok {
			log.Printf("   Accession Number: %v\n", accNum)
		}
		if orderID, ok := result["order_id"]; ok {
			log.Printf("   Order ID:         %v\n", orderID)
		}
		log.Println("\n🧪 TEST INFORMATION:")
		log.Printf("   Test Code:        %v\n", result["test_code"])
		log.Printf("   Test Name:        %v\n", result["test_name"])
		log.Printf("   Value:            %v %v\n", result["value"], result["units"])
		log.Printf("   Reference Range:  %v\n", result["reference_range"])
		log.Printf("   Abnormal Flags:   %v\n", result["abnormal_flags"])
		log.Printf("   Result Status:    %v\n", result["result_status"])
		log.Println("\n📨 MESSAGE INFORMATION:")
		if msgID, ok := result["message_id"]; ok {
			log.Printf("   Message ID:       %v\n", msgID)
		}
		if obsID, ok := result["observation_id"]; ok {
			log.Printf("   Observation ID:   %v\n", obsID)
		}
		if valType, ok := result["value_type"]; ok {
			log.Printf("   Value Type:       %v\n", valType)
		}
		if protocol, ok := result["protocol"]; ok {
			log.Printf("   Protocol:         %v\n", protocol)
		}
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
