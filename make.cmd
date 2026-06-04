@echo off
setlocal

if /I "%1"=="preprocess"   goto :preprocess
if /I "%1"=="index"        goto :index
if /I "%1"=="workerd"      goto :workerd
if /I "%1"=="test"         goto :test
if /I "%1"=="clean"        goto :clean
if /I "%1"=="build"        goto :build
if /I "%1"=="run"          goto :run
goto :build

:build
echo Building all binaries...
go build -o bin\preprocess.exe .\cmd\preprocess
if %errorlevel% neq 0 exit /b %errorlevel%
go build -o bin\index.exe .\cmd\index
if %errorlevel% neq 0 exit /b %errorlevel%
go build -o bin\workerd.exe .\cmd\workerd
if %errorlevel% equ 0 echo Build succeeded: bin\preprocess.exe, bin\index.exe, bin\workerd.exe
goto :end

:preprocess
echo Building and running preprocessing pipeline...
go build -o bin\preprocess.exe .\cmd\preprocess
if %errorlevel% neq 0 exit /b %errorlevel%
bin\preprocess.exe %2 %3 %4 %5 %6 %7 %8 %9
goto :end

:index
echo Building and running index pipeline...
go build -o bin\index.exe .\cmd\index
if %errorlevel% neq 0 exit /b %errorlevel%
bin\index.exe %2 %3 %4 %5 %6 %7 %8 %9
goto :end

:workerd
echo Building and starting worker daemon...
go build -o bin\workerd.exe .\cmd\workerd
if %errorlevel% neq 0 exit /b %errorlevel%
bin\workerd.exe
goto :end

:test
echo Running tests...
go test ./...
goto :end

:run
echo Running pre-built binaries...
if exist bin\workerd.exe (
    start "workerd" bin\workerd.exe
) else (
    echo Build workerd first: make.cmd build
)
if exist bin\preprocess.exe (
    bin\preprocess.exe
) else (
    echo Build preprocess first: make.cmd build
)
goto :end

:clean
echo Cleaning all artifacts...
if exist bin\          rmdir /s /q bin
if exist artifacts\    rmdir /s /q artifacts
goto :end

:end
endlocal
