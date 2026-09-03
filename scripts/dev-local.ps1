param(
    [switch]$NoAir
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

# 1. Ensure GOPATH/bin is in PATH for air
$gopath = & go env GOPATH
if ($gopath -and (Test-Path "$gopath\bin")) {
    if ($env:PATH -notlike "*$gopath\bin*") {
        $env:PATH = "$gopath\bin;" + $env:PATH
    }
}

# 2. Stop container API if running so port 3001 is free
Write-Host "Checking for container API on port 3001..." -ForegroundColor Cyan
try {
    $apiContainer = docker ps -q -f "name=local-mujeeb-api-1"
    if ($apiContainer) {
        Write-Host "Stopping container local-mujeeb-api-1 to free port 3001..." -ForegroundColor Yellow
        docker stop local-mujeeb-api-1 | Out-Null
    }
} catch {
    # Ignore docker error if docker is not running
}

# 3. Load deploy/local/.env
$envFile = Join-Path $repoRoot 'deploy/local/.env'
if (-not (Test-Path $envFile)) {
    $envFile = Join-Path $repoRoot '.env'
}

if (Test-Path $envFile) {
    Write-Host "Loading environment from $envFile..." -ForegroundColor Cyan
    Get-Content $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $parts = $line.Split("=", 2)
            $k = $parts[0].Trim()
            $v = $parts[1].Trim()
            [Environment]::SetEnvironmentVariable($k, $v, "Process")
        }
    }
}

# 4. Set Local Database URL and HTTP Address
$dbUser = if ($env:MUJEEB_POSTGRES_USERNAME) { $env:MUJEEB_POSTGRES_USERNAME } else { "mujeeb" }
$dbPass = if ($env:MUJEEB_POSTGRES_PASSWORD) { $env:MUJEEB_POSTGRES_PASSWORD } else { "password" }
$dbName = if ($env:MUJEEB_POSTGRES_DATABASE) { $env:MUJEEB_POSTGRES_DATABASE } else { "mujeeb24" }

$env:DATABASE_URL = "postgres://${dbUser}:${dbPass}@127.0.0.1:5433/${dbName}?sslmode=disable"
$env:HTTP_ADDR = ":3001"

Write-Host "========================================" -ForegroundColor Green
Write-Host " Mujeeb 24 - Local Development Server" -ForegroundColor Green
Write-Host " Port: http://127.0.0.1:3001" -ForegroundColor Green
Write-Host " DB:   127.0.0.1:5433 ($dbName)" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

# 5. Run air if available, else go run
$airPath = Get-Command air -ErrorAction SilentlyContinue
if ($airPath -and (-not $NoAir)) {
    Write-Host "Starting Air (Hot-Reload enabled)..." -ForegroundColor Yellow
    Set-Location $repoRoot
    & air
} else {
    Write-Host "Air not found or skipped. Running with 'go run ./cmd/api'..." -ForegroundColor Yellow
    Set-Location $repoRoot
    & go run ./cmd/api
}
