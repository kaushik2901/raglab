@echo off
setlocal

if /I "%1"=="workerd"      goto :workerd
if /I "%1"=="api"          goto :api
if /I "%1"=="test"         goto :test
if /I "%1"=="clean"        goto :clean
if /I "%1"=="build"        goto :build
if /I "%1"=="run"          goto :run
goto :build

:build
echo Building binaries...
go build -o bin\workerd.exe .\cmd\workerd
if %errorlevel% neq 0 exit /b %errorlevel%
go build -o bin\api.exe .\cmd\api
if %errorlevel% neq 0 exit /b %errorlevel%
if %errorlevel% equ 0 echo Build succeeded: bin\workerd.exe, bin\api.exe
goto :end

:workerd
echo Building and starting worker daemon...
go build -o bin\workerd.exe .\cmd\workerd
if %errorlevel% neq 0 exit /b %errorlevel%
bin\workerd.exe
goto :end

:api
echo Building and starting API server...
go build -o bin\api.exe .\cmd\api
if %errorlevel% neq 0 exit /b %errorlevel%
bin\api.exe
goto :end

:test
echo Running tests...
go test ./...
goto :end

:clean
echo Cleaning all artifacts...
if exist bin\          rmdir /s /q bin
goto :end

:end
endlocal
