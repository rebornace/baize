# 导出干净树到 _public_export，供推送到开源仓 baize
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Out = Join-Path $Root "_public_export"

if (Test-Path $Out) {
    Remove-Item -Recurse -Force $Out
}
New-Item -ItemType Directory -Path $Out | Out-Null

$excludeDirs = @(
    ".git", ".cursor", ".idea", "bin", "dist", "vendor",
    "_public_export", "_public_git",
    "docs\superpowers", ".superpowers", ".worktrees",
    "data"
)
$excludeFiles = @(
    ".env",
    "*.exe",
    "*.test"
)

function Test-ExcludedFile([string]$name) {
    foreach ($pat in $excludeFiles) {
        if ($name -like $pat) { return $true }
    }
    return $false
}

Get-ChildItem -Path $Root -Force | ForEach-Object {
    $name = $_.Name
    if ($name -eq ".git" -or $name -eq "_public_export" -or $name -eq "_public_git") { return }
    if ($excludeDirs -contains $name) { return }
    if (Test-ExcludedFile $name) { return }

    if ($name -eq "docs") {
        $docsOut = Join-Path $Out "docs"
        New-Item -ItemType Directory -Path $docsOut -Force | Out-Null
        Get-ChildItem (Join-Path $Root "docs") -Force | ForEach-Object {
            if ($_.Name -eq "superpowers") { return }
            Copy-Item $_.FullName -Destination (Join-Path $docsOut $_.Name) -Recurse -Force
        }
        return
    }

    if ($name -eq "configs") {
        $cfgOut = Join-Path $Out "configs"
        New-Item -ItemType Directory -Path $cfgOut -Force | Out-Null
        Get-ChildItem (Join-Path $Root "configs") -Force | ForEach-Object {
            if ($_.Name -eq "demo.local.yaml") { return }
            Copy-Item $_.FullName -Destination (Join-Path $cfgOut $_.Name) -Recurse -Force
        }
        return
    }

    if ($name -eq "examples") {
        $exOut = Join-Path $Out "examples"
        New-Item -ItemType Directory -Path $exOut -Force | Out-Null
        Get-ChildItem (Join-Path $Root "examples") -Force | ForEach-Object {
            if ($_.Name -eq "miaomiao") { return }
            Copy-Item $_.FullName -Destination (Join-Path $exOut $_.Name) -Recurse -Force
        }
        return
    }

    Copy-Item $_.FullName -Destination (Join-Path $Out $name) -Recurse -Force
}

Write-Host "已导出到: $Out"
Write-Host "下一步示例:"
Write-Host "  cd $Out"
Write-Host "  git init"
Write-Host "  git remote add origin https://github.com/rebornace/baize.git"
Write-Host '  git add .'
Write-Host '  git commit -m "release: 同步开源发布切片"'
Write-Host "  git branch -M main"
Write-Host "  git push -u origin main"
