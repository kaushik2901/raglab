@echo off
setlocal

set IMAGE=handbook-pipeline
set MOUNT=-v "%CD%:/workspace"
set RUNTIME_ENV=-e DATABASE_URL=postgres://rag:rag@host.docker.internal:5432/rag?sslmode=disable -e QDRANT_URL=http://host.docker.internal:6334

if /I "%1"=="preprocess"   goto :preprocess
if /I "%1"=="index"        goto :index
if /I "%1"=="workerd"      goto :workerd
if /I "%1"=="test"         goto :test
if /I "%1"=="clean"        goto :clean
goto :build

:build
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Building preprocessing binary...
docker run %MOUNT% %IMAGE% go build -o bin/preprocess ./cmd/preprocess
if %errorlevel% equ 0 echo Build succeeded: bin/preprocess
goto :end

:preprocess
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Building and running preprocessing pipeline...
docker run --rm %MOUNT% %RUNTIME_ENV% %IMAGE% sh -c "go build -o bin/preprocess ./cmd/preprocess && ./bin/preprocess"
goto :end

:index
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Building and running index pipeline...
docker run --rm %MOUNT% %RUNTIME_ENV% %IMAGE% sh -c "go build -o bin/index.exe ./cmd/index && ./bin/index.exe"
goto :end

:workerd
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Building and starting worker daemon...
docker run --rm %MOUNT% %RUNTIME_ENV% %IMAGE% sh -c "go build -o bin/workerd ./cmd/workerd && ./bin/workerd"
goto :end

:test
echo Building Docker image...
docker build -t %IMAGE% -f Dockerfile .
if %errorlevel% neq 0 exit /b %errorlevel%
echo Running tests...
docker run %MOUNT% %IMAGE% go test ./...
goto :end

:run
if not exist bin\preprocess goto :build
echo Running pre-built binary in Docker...
docker run --rm %MOUNT% %RUNTIME_ENV% %IMAGE% ./bin/preprocess
goto :end

:run-index
if not exist bin\index goto :build-index-check
echo Running pre-built index binary in Docker...
docker run --rm %MOUNT% %RUNTIME_ENV% %IMAGE% ./bin/index.exe
goto :end

:build-index-check
echo Build index first with: make.cmd index & goto :end

:clean
echo Cleaning all artifacts...
if exist bin\                rmdir /s /q bin
if exist output\             rmdir /s /q output
if exist .journal\           rmdir /s /q .journal
if exist .journal-index\     rmdir /s /q .journal-index
del /s /q *.exe 2>nul
goto :end

:end
endlocal
