package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	serverIP := "192.168.110.193"
	port := "7007"

	conn, err := net.Dial("tcp", serverIP+":"+port)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("Connected to server")

	hl7 := "MSH|^~\\&|HRJ-BIO|LAB|LIS|HOSPITAL|202602130930||ORU^R01|123456|P|2.3\r" +
		"PID|1||12345||DOE^JOHN\r" +
		"OBR|1||54321|TEST^Blood Test\r" +
		"OBX|1|NM|GLU^Glucose||5.6|mmol/L|3.9-6.1|N\r"

	// Wrap with MLLP
	packet := append([]byte{0x0B}, []byte(hl7)...)
	packet = append(packet, 0x1C, 0x0D)

	_, err = conn.Write(packet)
	if err != nil {
		panic(err)
	}

	fmt.Println("HL7 packet sent")
	time.Sleep(2 * time.Second)
}
