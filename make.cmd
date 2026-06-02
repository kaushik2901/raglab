@echo off
setlocal

if /I "%1"=="run"    goto :run
if /I "%1"=="clean"  goto :clean
if /I "%1"=="test"   goto :test
goto :build

:build
echo Building...
if not exist bin mkdir bin
go build -o bin\preprocess.exe .\cmd\preprocess
if %errorlevel% equ 0 echo Build succeeded: bin\preprocess.exe
goto :end

:run
call :build
if %errorlevel% equ 0 bin\preprocess.exe
goto :end

:clean
echo Cleaning...
if exist bin\      rmdir /s /q bin
if exist output\   rmdir /s /q output
if exist .journal\ rmdir /s /q .journal
goto :end

:test
echo Running tests...
go test .\...
goto :end

:end
endlocal
