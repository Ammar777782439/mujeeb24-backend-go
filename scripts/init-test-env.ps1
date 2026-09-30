param(
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$target = Join-Path $repoRoot 'deploy/local/.env'

if ((Test-Path $target) -and (-not $Force)) {
    throw "Refusing to replace existing $target. Use -Force only when you intentionally want new local secrets."
}

function New-HexSecret([int]$ByteCount) {
    $bytes = New-Object byte[] $ByteCount
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    }
    finally {
        $rng.Dispose()
    }
    return ([System.BitConverter]::ToString($bytes) -replace '-', '').ToLowerInvariant()
}

$mujeebDbPassword = New-HexSecret 24

@"
MUJEEB_POSTGRES_DATABASE=mujeeb24
MUJEEB_POSTGRES_USERNAME=mujeeb
MUJEEB_POSTGRES_PASSWORD=$mujeebDbPassword
MUJEEB_HOST_PORT=3001

SOCIALAPI_BASE_URL=https://api.social-api.ai
SOCIALAPI_API_KEY=
SOCIALAPI_WEBHOOK_SECRET=
CHANNEL_PROVISIONING_ENABLED=false
CHANNEL_PROVISIONING_REDIRECT_URI=
AUTOREPLY_ENABLED=false
LLM_ENABLED=false
LLM_BASE_URL=
LLM_API_KEY=
LLM_MODEL=
CLOUDFLARE_TUNNEL_TOKEN=
"@ | Set-Content -Path $target -Encoding ascii -NoNewline

Write-Host "Created $target with new local-only Mujeeb and PostgreSQL secrets."
Write-Host 'External integrations remain disabled. Do not commit this file.'
