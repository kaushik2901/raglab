param(
    [ValidateSet('build', 'run', 'clean', 'test')]
    [string]$Target = 'build'
)

function Invoke-Build {
    Write-Host "Building..." -ForegroundColor Cyan
    $null = New-Item -ItemType Directory -Force -Path "bin"
    go build -o bin/preprocess.exe ./cmd/preprocess
    if ($?) { Write-Host "Build succeeded: bin/preprocess.exe" -ForegroundColor Green }
}

function Invoke-Run {
    Invoke-Build
    if ($?) {
        Write-Host "Running preprocess pipeline..." -ForegroundColor Cyan
        & "bin/preprocess.exe"
    }
}

function Invoke-Clean {
    Write-Host "Cleaning..." -ForegroundColor Cyan
    $dirs = @("bin", "output", ".journal")
    foreach ($d in $dirs) {
        if (Test-Path -LiteralPath $d) {
            Remove-Item -LiteralPath $d -Recurse -Force
            Write-Host "  Removed $d/" -ForegroundColor Yellow
        }
    }
}

function Invoke-Test {
    Write-Host "Running tests..." -ForegroundColor Cyan
    go test ./...
}

switch ($Target) {
    'build' { Invoke-Build }
    'run'   { Invoke-Run }
    'clean' { Invoke-Clean }
    'test'  { Invoke-Test }
}
