@echo off
cd /d C:\Users\darle\Documents\lightbaseERMGateway
git pull
go build -o server.exe ./cmd/server 
start "LightbaseERMGateway" cmd /k server.exe
