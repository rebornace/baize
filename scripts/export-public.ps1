# 导出干净树到 _public_export，供推送到开源仓 baize
# 两道保险：
#   1) 只复制 git 已跟踪文件——被 .gitignore 忽略的本地私有内容永不进入切片；
#   2) 导出后做敏感词/密钥硬校验，命中即中止，绝不推送。
# 私人敏感词写在本机的 scripts/.export-secret-patterns.txt（已被 gitignore，勿提交），
# 每行一个正则，# 开头为注释。脚本内置通用高置信密钥特征，无需词表也会校验。
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

# ---------- 第二道保险：敏感信息硬校验（命中即中止） ----------

# 内置：高置信真实密钥特征（阈值刻意调高，避免误伤测试占位值）
$builtinPatterns = @(
    'sk-[A-Za-z0-9]{20,}',
    'ghp_[A-Za-z0-9]{36}',
    'github_pat_[A-Za-z0-9_]{20,}',
    'AKIA[0-9A-Z]{16}',
    'xox[baprs]-[A-Za-z0-9-]{10,}',
    '-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----'
)

# 用户私人词表（本机、被 gitignore）
$patternFile = Join-Path $Root "scripts\.export-secret-patterns.txt"
$userPatterns = @()
if (Test-Path $patternFile) {
    $userPatterns = Get-Content $patternFile -Encoding UTF8 |
        ForEach-Object { $_.Trim() } |
        Where-Object { $_ -and -not $_.StartsWith("#") }
    Write-Host "已加载私人敏感词表: $($userPatterns.Count) 条规则"
}
else {
    Write-Warning "未找到 scripts/.export-secret-patterns.txt：仅执行内置密钥校验，建议补充本机私人词表（该文件已被 gitignore）。"
}
$allPatterns = $builtinPatterns + $userPatterns

$violations = New-Object System.Collections.Generic.List[string]
Get-ChildItem -Path $Out -Recurse -File | ForEach-Object {
    $rel = $_.FullName.Substring($Out.Length + 1) -replace '\\', '/'

    # 路径本身也要查私人词表（防止文件名泄密）
    foreach ($p in $userPatterns) {
        if ($rel -imatch $p) {
            $violations.Add("路径命中 /$p/  -> $rel")
        }
    }

    # 跳过二进制（含 NUL 字节）
    $bytes = [System.IO.File]::ReadAllBytes($_.FullName)
    if ($bytes -contains 0) { return }
    $text = [System.Text.Encoding]::UTF8.GetString($bytes)

    foreach ($p in $allPatterns) {
        $m = [regex]::Match($text, $p, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
        if ($m.Success) {
            $lineNo = ($text.Substring(0, $m.Index) -split "`n").Count
            $preview = $m.Value
            if ($preview.Length -gt 24) { $preview = $preview.Substring(0, 24) + "..." }
            $violations.Add("内容命中 /$p/  -> ${rel}:$lineNo  ($preview)")
        }
    }
}

if ($violations.Count -gt 0) {
    Write-Host ""
    Write-Host "导出中止：发现 $($violations.Count) 处疑似敏感信息，请处理后重新导出：" -ForegroundColor Red
    $violations | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
    Write-Host ""
    Write-Host "提示：私人系统名/域名等加入 scripts/.export-secret-patterns.txt；真实配置应放在被 gitignore 的本地文件中。"
    exit 1
}

Write-Host "敏感信息校验通过（内置 $($builtinPatterns.Count) 条 + 私人 $($userPatterns.Count) 条规则）。"
Write-Host ""
Write-Host "下一步示例（提交信息请与 real 仓一致、如实描述功能，不要写开源切片/发布切片之类措辞）:"
Write-Host "  cd $Out"
Write-Host "  git init"
Write-Host "  git remote add origin https://github.com/rebornace/baize.git"
Write-Host '  git add .'
Write-Host '  git commit -m "feat: 与 real 仓一致的真实功能描述"   # 切勿写“开源切片/发布切片”'
Write-Host "  git branch -M main"
Write-Host "  git push -u origin main --force-with-lease"
