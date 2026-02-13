# lightbaseERMGateway



# Allow TCP port 7007 inbound from any device on private network
New-NetFirewallRule -DisplayName "Allow HL7 TCP 7007" -Direction Inbound -Protocol TCP -LocalPort 7007 -Action Allow


Get-NetFirewallRule -DisplayName "Allow HL7 TCP 7007"
