package config

// Control characters
const (
	VT = 0x0B // Start Block
	FS = 0x1C // End Block
	CR = 0x0D // Carriage Return
	LF = 0x0A // Line Feed
)

// ASTM control characters
const (
	ENQ = 0x05 // Enquiry
	ACK = 0x06 // Acknowledge
	NAK = 0x15 // Negative Acknowledge
	STX = 0x02 // Start of Text
	ETX = 0x03 // End of Text
	ETB = 0x17 // End of Transmission Block
	EOT = 0x04 // End of Transmission
)

// Server configuration
const (
	PCIP             = "192.168.1.193"
	ListenPort       = "7007"
	DebugMode        = true
	LogToTerminal    = true
	ASTMComPort      = "COM1"
	ASTMBaudRate     = 115200
	ASTMTCPPort      = "5000"
	ExternalSaverURL = "https://api-dev.lightbasemr.com"
	LABSLUG          = "darlez-dev"
)
