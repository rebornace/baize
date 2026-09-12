# 导出干净树到 _public_export，供推送到开源仓 baize
# 只复制 git 已跟踪文件，被 .gitignore 忽略的本地私有内容
# （如 configs/default.local.yaml、examples/local 下的真实 spec、.env、data 等）永不进入切片。
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Out = Join-Path $Root "_public_export"

if (Test-Path $Out) {
    Remove-Item -Recurse -Force $Out
}
New-Item -ItemType Directory -Path $Out -Force | Out-Null

# 路径前缀（相对路径以这些片段开头则排除）
$excludePrefixes = @(
    ".cursor",
    ".idea",
    ".superpowers",
    ".worktrees",
    "_public_export",
    "_public_git",
    "bin/",
    "dist/",
    "vendor/",
    "data/",
    "docs/superpowers/"
)

# 精确文件名 / 通配（仅作用于文件，.env.example 等不受影响）
$excludeFileNames = @(".env")
$excludeFilePatterns = @("*.exe", "*.test")

function Test-Excluded([string]$rel) {
    $normalized = $rel -replace '\\', '/'
    foreach ($p in $excludePrefixes) {
        if ($normalized.StartsWith($p)) { return $true }
    }
    $name = Split-Path $normalized -Leaf
    if ($excludeFileNames -contains $name) { return $true }
    foreach ($pat in $excludeFilePatterns) {
        if ($name -like $pat) { return $true }
    }
    # 双保险：任何本地私有覆盖/真实 spec 一律不导出
    if ($normalized -eq "configs/default.local.yaml") { return $true }
    if ($normalized -like "*.local.yaml" -and $normalized -notlike "*.example") { return $true }
    return $false
}

Push-Location $Root
try {
    $tracked = git ls-files
}
finally {
    Pop-Location
}

$count = 0
foreach ($rel in $tracked) {
    if (-not $rel) { continue }
    if (Test-Excluded $rel) { continue }

    $src = Join-Path $Root $rel
    if (-not (Test-Path $src -PathType Leaf)) { continue }

    $dst = Join-Path $Out $rel
    $dstDir = Split-Path $dst -Parent
    if (-not (Test-Path $dstDir)) {
        New-Item -ItemType Directory -Path $dstDir -Force | Out-Null
    }
    Copy-Item $src -Destination $dst -Force
    $count++
}

Write-Host "已导出 $count 个已跟踪文件到: $Out"
Write-Host "下一步示例:"
Write-Host "  cd $Out"
Write-Host "  git init"
Write-Host "  git remote add origin https://github.com/rebornace/baize.git"
Write-Host '  git add .'
Write-Host '  git commit -m "release: 同步开源发布切片"'
Write-Host "  git branch -M main"
Write-Host "  git push -u origin main --force-with-lease"
