@echo off
setlocal

set IMAGE=handbook-pipeline
set MOUNT=-v "%CD%:/workspace"

if /I "%1"=="clean"        goto :clean
if /I "%1"=="clean-index"  goto :clean-index
if /I "%1"=="test"         goto :test
if /I "%1"=="build-index"  goto :build-index
if /I "%1"=="run-index"    goto :run-index
if /I "%1"=="run"          goto :run
goto :build

:build
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Building...
docker run --rm %MOUNT% %IMAGE% go build -o bin/preprocess ./cmd/preprocess
if %errorlevel% equ 0 echo Build succeeded: bin/preprocess
goto :end

:run
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
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
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Running tests...
docker run --rm %MOUNT% %IMAGE% go test ./...
goto :end

:build-index
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Building index...
docker run --rm %MOUNT% %IMAGE% go build -o bin/index.exe ./cmd/index
if %errorlevel% equ 0 echo Build succeeded: bin/index.exe
goto :end

:run-index
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Running index...
docker run --rm %MOUNT% %IMAGE% sh -c "go build -o bin/index.exe ./cmd/index && ./bin/index.exe"
goto :end

:clean-index
echo Cleaning index journal...
if exist .journal-index\ rmdir /s /q .journal-index
goto :end

:end
endlocal
