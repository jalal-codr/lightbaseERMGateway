package main

import (
	"log"
	"net"
	"strings"

	"lightbaseEMRProxy/internal/config"
	"lightbaseEMRProxy/internal/protocol/astm"
	"lightbaseEMRProxy/internal/protocol/hl7"
)

func main() {
	log.Println("🚀 Starting HL7 TCP/IP Server (Listening for LIS connections)")
	log.Println(strings.Repeat("=", 60))
	fullAddress := config.PCIP + ":" + config.ListenPort
	log.Printf("Listening on %s for incoming LIS connections...\n", fullAddress)

	printLocalIPs()

	// Start ASTM serial listener (non-blocking)
	go astm.StartSerialListener()

	// Start ASTM TCP listener (non-blocking)
	go astm.StartTCPListener()

	// Start HL7 TCP server (blocks)
	hl7.StartServer(fullAddress)
}

func printLocalIPs() {
	log.Println("\n📡 This Computer's IP Addresses:")
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Println("   Could not get local IPs:", err)
		return
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				log.Printf("   ℹ️  %s\n", ipnet.IP.String())
			}
		}
	}
	log.Println()
}
