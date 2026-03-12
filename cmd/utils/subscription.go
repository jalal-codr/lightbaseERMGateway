package utils

import (
	"encoding/json"
	"io"
	"lightbaseEMRProxy/internal/config"
	"log"
	"net/http"
	"os"
)

func CheckSubscription() {
	url := config.ExternalSaverURL + "/getsubscription-status?slug=" + config.LABSLUG

	resp, err := http.Get(url)
	if err != nil {
		log.Println("Failed to fetch subscription:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Print raw response for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read response body:", err)
		os.Exit(1)
	}
	log.Println("Raw response:", string(body))

	var result map[string]bool
	if err := json.Unmarshal(body, &result); err != nil {
		log.Println("Failed to decode subscription response:", err)
		os.Exit(1)
	}

	if !result["active"] {
		log.Println("No active subscription found for:", config.LABSLUG)
		os.Exit(1)
	}

	log.Println("Subscription active, starting server...")
}
