package hl7

import (
	"fmt"
	"strings"
	"time"

	"lightbaseEMRProxy/internal/config"
)

// GenerateACK creates an HL7 acknowledgment message
func GenerateACK(originalMessage string) string {
	originalMessage = strings.ReplaceAll(originalMessage, "\r\n", "\r")
	segments := strings.Split(originalMessage, string(config.CR))

	var mshFields []string
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if strings.HasPrefix(segment, "MSH") {
			mshFields = strings.Split(segment, "|")
			break
		}
	}

	if len(mshFields) < 10 {
		return ""
	}

	fieldSeparator := "|"
	encodingChars := getField(mshFields, 1)
	sendingApp := getField(mshFields, 2)
	sendingFacility := getField(mshFields, 3)
	receivingApp := getField(mshFields, 4)
	receivingFacility := getField(mshFields, 5)
	messageControlID := getField(mshFields, 9)

	timestamp := time.Now().Format("20060102150405")

	ack := fmt.Sprintf("MSH%s%s%s%s%s%s%s%s%s%sACK%s%s%sAL%s",
		fieldSeparator,
		encodingChars,
		fieldSeparator,
		receivingApp,
		fieldSeparator,
		receivingFacility,
		fieldSeparator,
		sendingApp,
		fieldSeparator,
		sendingFacility,
		fieldSeparator,
		timestamp,
		fieldSeparator,
		fieldSeparator,
		fieldSeparator,
	)
	ack += string(config.CR)

	ack += fmt.Sprintf("MSA%sAA%s%s",
		fieldSeparator,
		fieldSeparator,
		messageControlID,
	)

	return ack
}
