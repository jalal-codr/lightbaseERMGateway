package main

import (
	"fmt"
	"net"
)

func main() {
	server := "192.168.110.193:7007"
	conn, err := net.Dial("tcp", server)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	hl7 := "MSH|^~\\&|HRJ-BIO|LAB|LIS|HOSP|202602130930||ORU^R01|123456|P|2.3\r" +
		"PID|1||12345||DOE^JOHN\r" +
		"OBR|1||54321|TEST^Blood Test\r" +
		"OBX|1|NM|GLU^Glucose||5.6|mmol/L|3.9-6.1|N\r"

	// Wrap with MLLP
	packet := append([]byte{0x0B}, []byte(hl7)...)
	packet = append(packet, 0x1C, 0x0D)

	conn.Write(packet)
	fmt.Println("Packet sent")
}
