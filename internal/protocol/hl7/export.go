package hl7

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"lightbaseEMRProxy/types"
	"log"
	"net/http"
	"time"
)

// SendToExternalSaver sends parsed HL7 data to an external persistence service
func SendToExternalSaver(payload types.HL7Message, endpoint string) error {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal HL7 payload: %w", err)
	}

	log.Printf("[HL7 Sender] Sending to %s\nBody: %s", endpoint, string(jsonBody))

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

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	log.Printf("[HL7 Sender] Response status: %d\nBody: %s", resp.StatusCode, string(rawBody))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("external saver returned non-2xx status: %d — %s", resp.StatusCode, string(rawBody))
	}

	return nil
}
