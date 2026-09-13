# Trial launcher: demo.yaml + optional default.local.yaml (real LLM) + demo.local.yaml.
# Usage: .\demo.cmd
& (Join-Path $PSScriptRoot "baize.ps1") demo @args
