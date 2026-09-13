# Production launcher (minimal.yaml). Requires BAIZE_API_KEY.
# Usage: .\start.cmd
& (Join-Path $PSScriptRoot "baize.ps1") start @args
