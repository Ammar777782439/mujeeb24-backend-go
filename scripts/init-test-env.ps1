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
$chatwootDbPassword = New-HexSecret 24
$redisPassword = New-HexSecret 24
$railsSecret = New-HexSecret 64

@"
MUJEEB_POSTGRES_DATABASE=mujeeb24
MUJEEB_POSTGRES_USERNAME=mujeeb
MUJEEB_POSTGRES_PASSWORD=$mujeebDbPassword
MUJEEB_HOST_PORT=3001

POSTGRES_DATABASE=chatwoot
POSTGRES_USERNAME=chatwoot
POSTGRES_PASSWORD=$chatwootDbPassword
REDIS_PASSWORD=$redisPassword
SECRET_KEY_BASE=$railsSecret
FRONTEND_URL=http://localhost:3002
CHATWOOT_HOST_PORT=3002
ACTIVE_STORAGE_SERVICE=local
ENABLE_ACCOUNT_SIGNUP=true

SOCIALAPI_BASE_URL=https://api.social-api.ai
SOCIALAPI_API_KEY=
SOCIALAPI_WEBHOOK_SECRET=
CHATWOOT_BASE_URL=http://chatwoot-rails:3000
CHATWOOT_API_TOKEN=
CHATWOOT_PLATFORM_API_TOKEN=
CHATWOOT_WEBHOOK_SECRET=
CHATWOOT_MIRROR_ENABLED=false
CHATWOOT_AUTOREPLY_ENABLED=false
CHATWOOT_PROVISIONING_ENABLED=false
LLM_ENABLED=false
LLM_BASE_URL=
LLM_API_KEY=
LLM_MODEL=
CLOUDFLARE_TUNNEL_TOKEN=
"@ | Set-Content -Path $target -Encoding ascii -NoNewline

Write-Host "Created $target with new local-only Mujeeb, PostgreSQL, Redis, and Chatwoot secrets."
Write-Host 'External integrations remain disabled. Do not commit this file.'
