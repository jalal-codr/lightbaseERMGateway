# lightbaseEMRProxy

A Go-based gateway server for receiving and processing HL7 and ASTM laboratory results from LIS (Laboratory Information Systems).

## Features

- HL7 TCP/IP server for receiving lab results
- ASTM E1394 protocol support (both serial and TCP)
- Automatic message parsing and acknowledgment
- Real-time result logging

## Project Structure

```
lightbaseEMRProxy/
├── cmd/
│   └── server/          # Application entry point
│       └── main.go
├── internal/
│   ├── config/          # Configuration constants
│   │   └── config.go
│   ├── protocol/
│   │   ├── hl7/         # HL7 protocol implementation
│   │   │   ├── server.go
│   │   │   ├── parser.go
│   │   │   └── ack.go
│   │   └── astm/        # ASTM protocol implementation
│   │       ├── serial.go
│   │       ├── tcp.go
│   │       └── parser.go
│   └── logger/          # Result logging
│       └── logger.go
├── go.mod
└── README.md
```

## Building

```bash
go build -o lightbaseEMRProxy.exe ./cmd/server
```

## Configuration

Edit `internal/config/config.go` to configure:
- Server IP and ports
- ASTM serial port settings
- Debug mode and logging options

## Firewall Configuration (Windows)

Allow TCP port 7007 inbound:

```powershell
New-NetFirewallRule -DisplayName "Allow HL7 TCP 7007" -Direction Inbound -Protocol TCP -LocalPort 7007 -Action Allow
```

Verify the rule:

```powershell
Get-NetFirewallRule -DisplayName "Allow HL7 TCP 7007"
```

## Running

**Option 1: Run directly with Go (recommended for development)**
```bash
go run ./cmd/server
```

**Option 2: Build and run the executable**
```bash
go build -o lightbaseEMRProxy.exe ./cmd/server
./lightbaseEMRProxy.exe
```

**Option 3: Use Go install (installs to $GOPATH/bin)**
```bash
go install ./cmd/server
lightbaseEMRProxy
```

## Protocols Supported

- HL7 v2.x over TCP/IP (MLLP framing)
- ASTM E1394 over serial (COM port)
- ASTM E1394 over TCP/IP
