@echo off
cd /d C:\Users\darle\Documents\lightbaseERMGateway
git pull

REM Clean build artifacts except server.exe
for /f "delims=" %%f in ('dir /b /a-d ^| findstr /v /i "server.exe"') do del "%%f"
for /d %%d in (*) do rd /s /q "%%d"

go build -o server.exe ./cmd/server
start "LightbaseERMGateway" cmd /k server.exe