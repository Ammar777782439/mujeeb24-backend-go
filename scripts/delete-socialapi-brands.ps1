# PowerShell script to clean up / delete all SocialAPI brands

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

# 1. Load environment variables
$envFile = Join-Path $repoRoot 'deploy/local/.env'
if (-not (Test-Path $envFile)) {
    $envFile = Join-Path $repoRoot '.env'
}

$apiKey = ""
$baseUrl = "https://api.social-api.ai"

if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $parts = $line.Split("=", 2)
            $k = $parts[0].Trim()
            $v = $parts[1].Trim()
            if ($k -eq "SOCIALAPI_API_KEY") { $apiKey = $v }
            if ($k -eq "SOCIALAPI_BASE_URL") { $baseUrl = $v }
        }
    }
}

if (-not $apiKey) {
    Write-Host "Error: SOCIALAPI_API_KEY is not set in $envFile" -ForegroundColor Red
    exit 1
}

$headers = @{
    "Authorization" = "Bearer $apiKey"
    "Content-Type"  = "application/json"
}

Write-Host "========================================" -ForegroundColor Cyan
Write-Host " SocialAPI - Delete All Brands" -ForegroundColor Cyan
Write-Host " Base URL: $baseUrl" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan

# 2. Fetch all brands
try {
    $response = Invoke-RestMethod -Uri "$baseUrl/v1/brands" -Headers $headers -Method Get
    $brands = $response.data
    if (-not $brands -or $brands.Count -eq 0) {
        Write-Host "No brands found in your SocialAPI account." -ForegroundColor Green
        exit 0
    }

    Write-Host "Found $($brands.Count) brand(s):" -ForegroundColor Yellow
    foreach ($brand in $brands) {
        Write-Host " - ID: $($brand.id), Name: $($brand.name), Accounts: $($brand.accounts_count)"
    }
} catch {
    Write-Host "Failed to fetch brands: $_" -ForegroundColor Red
    exit 1
}

# 3. Delete each brand
Write-Host "`nDeleting brands..." -ForegroundColor Yellow
foreach ($brand in $brands) {
    $brandId = $brand.id
    $brandName = $brand.name
    Write-Host "Deleting brand: $brandName ($brandId)..." -NoNewline
    try {
        $delResponse = Invoke-WebRequest -Uri "$baseUrl/v1/brands/$brandId" -Headers $headers -Method Delete -UseBasicParsing
        if ($delResponse.StatusCode -ge 200 -and $delResponse.StatusCode -lt 300) {
            Write-Host " [DELETED]" -ForegroundColor Green
        } else {
            Write-Host " [Status: $($delResponse.StatusCode)]" -ForegroundColor Yellow
        }
    } catch {
        Write-Host " [FAILED: $_]" -ForegroundColor Red
    }
}

# 4. Confirm final count
Write-Host "`nVerifying remaining brands..." -ForegroundColor Cyan
try {
    $verify = Invoke-RestMethod -Uri "$baseUrl/v1/brands" -Headers $headers -Method Get
    $remaining = $verify.data
    $count = if ($remaining) { $remaining.Count } else { 0 }
    Write-Host "Remaining brands count: $count" -ForegroundColor Green
} catch {
    Write-Host "Verification query failed: $_" -ForegroundColor Yellow
}
