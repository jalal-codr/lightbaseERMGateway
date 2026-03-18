@echo off
cd /d C:\Users\darle\Documents\lightbaseERMGateway

echo Pulling latest changes...
git pull
if %errorlevel% neq 0 (
    echo Git pull failed. Aborting.
    pause
    exit /b 1
)

echo Building server...
go build -o server.exe ./cmd/server
if %errorlevel% neq 0 (
    echo Build failed. Aborting.
    pause
    exit /b 1
)

echo Starting server...
start "LightbaseERMGateway" cmd /k server.exe