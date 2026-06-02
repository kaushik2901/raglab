@echo off
setlocal

set IMAGE=handbook-pipeline
set MOUNT=-v "%CD%:/workspace"

echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%

if /I "%1"=="run"    goto :run
if /I "%1"=="clean"  goto :clean
if /I "%1"=="test"   goto :test
goto :build

:build
echo Building...
docker run --rm %MOUNT% %IMAGE% go build -o bin/preprocess ./cmd/preprocess
if %errorlevel% equ 0 echo Build succeeded: bin/preprocess
goto :end

:run
echo Running...
docker run --rm %MOUNT% %IMAGE% sh -c "go build -o bin/preprocess ./cmd/preprocess && ./bin/preprocess"
goto :end

:clean
echo Cleaning...
if exist bin\      rmdir /s /q bin
if exist output\   rmdir /s /q output
if exist .journal\ rmdir /s /q .journal
goto :end

:test
echo Running tests...
docker run --rm %MOUNT% %IMAGE% go test ./...
goto :end

:end
endlocal
