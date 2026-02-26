package hl7

import (
	"bytes"
	"encoding/json"
	"fmt"
	"lightbaseEMRProxy/types"
	"net/http"
	"time"
)

// SendToExternalSaver sends parsed HL7 data to an external persistence service
func SendToExternalSaver(payload types.HL7Message, endpoint string) error {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal HL7 payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Source", "hl7-bridge")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("external saver request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("external saver returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}
