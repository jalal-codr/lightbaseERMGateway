@echo off
cd /d C:\Users\darle\Documents\lightbaseERMGateway
git pull

REM Build first
go build -o server.exe ./cmd/server

if %errorlevel% neq 0 (
    echo Build failed. Aborting cleanup.
    pause
    exit /b 1
)

REM Delete all files except server.exe
for /f "delims=" %%f in ('dir /b /a-d') do (
    if /i not "%%f"=="server.exe" del /f /q "%%f"
)

REM Delete all folders except .git
for /d %%d in (*) do (
    if /i not "%%d"==".git" rd /s /q "%%d"
)

start "LightbaseERMGateway" cmd /k server.exe