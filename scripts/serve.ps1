# Windows launcher: loads .env (if present), rebuilds the embedded Chat UI when
# web/chat sources are newer than internal/ui/dist, then builds and runs baize.
#
# Examples:
#   .\serve.cmd
#   .\serve.cmd -config configs\config.local.yaml
#   .\serve.cmd -SkipUI          # go-only rebuild (use when UI is unchanged)
#   .\serve.cmd -ForceUI         # always npm run build before go build
# Env: BAIZE_SKIP_UI=1 / BAIZE_FORCE_UI=1
$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $Root

# Split launcher flags from baize serve args.
$baizeArgs = New-Object System.Collections.Generic.List[string]
$skipUI = $false
$forceUI = $false
foreach ($a in $args) {
    $s = [string]$a
    if ($s -match '^(?i)(-SkipUI|--skip-ui)$') {
        $skipUI = $true
    } elseif ($s -match '^(?i)(-ForceUI|--force-ui)$') {
        $forceUI = $true
    } else {
        [void]$baizeArgs.Add($s)
    }
}
if ($env:BAIZE_SKIP_UI -eq '1') { $skipUI = $true }
if ($env:BAIZE_FORCE_UI -eq '1') { $forceUI = $true }

function Get-NewestWriteTimeUtc([string[]]$Paths) {
    $newest = [datetime]::MinValue
    foreach ($p in $Paths) {
        if (-not (Test-Path -LiteralPath $p)) { continue }
        Get-ChildItem -LiteralPath $p -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
            if ($_.LastWriteTimeUtc -gt $newest) { $newest = $_.LastWriteTimeUtc }
        }
    }
    return $newest
}

function Test-ChatUINeedsRebuild {
    $distDir = Join-Path $Root "internal\ui\dist"
    $distIndex = Join-Path $distDir "index.html"
    if (-not (Test-Path -LiteralPath $distIndex)) { return $true }

    $srcRoots = @(
        (Join-Path $Root "web\chat\src"),
        (Join-Path $Root "web\chat\index.html"),
        (Join-Path $Root "web\chat\package.json"),
        (Join-Path $Root "web\chat\package-lock.json"),
        (Join-Path $Root "web\chat\vite.config.ts"),
        (Join-Path $Root "web\chat\tsconfig.json"),
        (Join-Path $Root "web\chat\tsconfig.node.json")
    )
    $srcNewest = Get-NewestWriteTimeUtc $srcRoots
    $distNewest = Get-NewestWriteTimeUtc @($distDir)
    if ($srcNewest -eq [datetime]::MinValue) { return $false }
    return ($srcNewest -gt $distNewest)
}

function Ensure-ChatUI([bool]$Force) {
    $chatDir = Join-Path $Root "web\chat"
    if (-not (Test-Path -LiteralPath (Join-Path $chatDir "package.json"))) {
        Write-Host "web/chat missing; skipping UI build"
        return
    }
    if (-not $Force -and -not (Test-ChatUINeedsRebuild)) {
        Write-Host "chat UI up to date (internal/ui/dist newer than web/chat sources)"
        return
    }

    # Prefer npm.cmd on Windows. Node's npm.ps1 re-parses the caller's statement
    # when Line is set; `& npm run build` becomes `pm run build` (drops "& n")
    # and fails with: Unknown command: "pm".
    $npmCmd = Get-Command npm.cmd -ErrorAction SilentlyContinue
    $npm = if ($npmCmd) { $npmCmd } else { Get-Command npm -ErrorAction SilentlyContinue }
    if (-not $npm) {
        Write-Error @"
npm not found, but the Chat UI needs a rebuild (web/chat is newer than internal/ui/dist).
Install Node.js 20+ and ensure npm is on PATH, then re-run .\serve.cmd
Or pass -SkipUI / set BAIZE_SKIP_UI=1 to reuse the existing embedded UI (may be stale).
"@
        exit 1
    }

    Write-Host "npm run build  (cwd=$chatDir) -> internal/ui/dist"
    Push-Location $chatDir
    try {
        # Pass args as an array so nothing depends on statement-text reparsing.
        & $npm.Source @('run', 'build')
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
        Pop-Location
    }
}

# 把整个进程树放进一个 Job Object，并设置「句柄关闭即终止全部进程」。
# 这样当本外壳（powershell/cmd）以任何方式结束——Ctrl+C 之外，也包括被
# 外部强制终止——go build、baize.exe 等子进程都会被系统连带结束，不会
# 留下仍占用端口的孤立进程。失败时仅告警并继续，不阻断正常启动。
$jobSource = @'
using System;
using System.Runtime.InteropServices;

public static class BaizeJob
{
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode)]
    private static extern IntPtr CreateJobObject(IntPtr lpJobAttributes, string lpName);

    [DllImport("kernel32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetInformationJobObject(IntPtr hJob, int infoClass, IntPtr info, uint length);

    [DllImport("kernel32.dll")]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool AssignProcessToJobObject(IntPtr hJob, IntPtr hProcess);

    [DllImport("kernel32.dll")]
    private static extern IntPtr GetCurrentProcess();

    [StructLayout(LayoutKind.Sequential)]
    private struct BasicLimit
    {
        public long PerProcessUserTimeLimit;
        public long PerJobUserTimeLimit;
        public uint LimitFlags;
        public UIntPtr MinimumWorkingSetSize;
        public UIntPtr MaximumWorkingSetSize;
        public uint ActiveProcessLimit;
        public UIntPtr Affinity;
        public uint PriorityClass;
        public uint SchedulingClass;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct IOCounters
    {
        public ulong ReadOperationCount;
        public ulong WriteOperationCount;
        public ulong OtherOperationCount;
        public ulong ReadTransferCount;
        public ulong WriteTransferCount;
        public ulong OtherTransferCount;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct ExtendedLimit
    {
        public BasicLimit BasicLimitInformation;
        public IOCounters IoInfo;
        public UIntPtr ProcessMemoryLimit;
        public UIntPtr JobMemoryLimit;
        public UIntPtr PeakProcessMemoryUsed;
        public UIntPtr PeakJobMemoryUsed;
    }

    private const int InfoExtendedLimit = 9;
    private const uint LimitKillOnJobClose = 0x00002000;

    public static IntPtr Handle;

    public static void Attach()
    {
        Handle = CreateJobObject(IntPtr.Zero, null);
        if (Handle == IntPtr.Zero) throw new Exception("CreateJobObject failed");

        ExtendedLimit info = new ExtendedLimit();
        info.BasicLimitInformation.LimitFlags = LimitKillOnJobClose;
        int length = Marshal.SizeOf(typeof(ExtendedLimit));
        IntPtr ptr = Marshal.AllocHGlobal(length);
        try
        {
            Marshal.StructureToPtr(info, ptr, false);
            if (!SetInformationJobObject(Handle, InfoExtendedLimit, ptr, (uint)length))
                throw new Exception("SetInformationJobObject failed");
        }
        finally
        {
            Marshal.FreeHGlobal(ptr);
        }

        if (!AssignProcessToJobObject(Handle, GetCurrentProcess()))
            throw new Exception("AssignProcessToJobObject failed");
    }
}
'@

try {
    if (-not ("BaizeJob" -as [type])) {
        Add-Type -TypeDefinition $jobSource -Language CSharp
    }
    [BaizeJob]::Attach()
} catch {
    Write-Host "job-object attach skipped: $($_.Exception.Message)"
}

if (-not $env:GOPROXY) {
    $env:GOPROXY = "https://goproxy.cn,direct"
}
if (-not $env:GOSUMDB) {
    $env:GOSUMDB = "sum.golang.google.cn"
}

# serve.cmd starts a new PowerShell that may still have a stale PATH from an
# older terminal. Prefer `go` already on PATH; if missing, merge User+Machine
# PATH (and User GOROOT\bin) from the registry so a normal Go install works
# without hardcoding a machine-specific SDK folder.
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $userRoot = [Environment]::GetEnvironmentVariable("GOROOT", "User")
    if (-not $userRoot) {
        $userRoot = [Environment]::GetEnvironmentVariable("GOROOT", "Machine")
    }
    $prefix = @()
    if ($userRoot) {
        $prefix += (Join-Path $userRoot "bin")
        if (-not $env:GOROOT) { $env:GOROOT = $userRoot }
    }
    $env:Path = (@($prefix) + @($userPath, $machinePath, $env:Path) | Where-Object { $_ }) -join ";"
    $userToolchain = [Environment]::GetEnvironmentVariable("GOTOOLCHAIN", "User")
    if ($userToolchain -and -not $env:GOTOOLCHAIN) {
        $env:GOTOOLCHAIN = $userToolchain
    }
    $go = Get-Command go -ErrorAction SilentlyContinue
}
if (-not $go) {
    Write-Error "go not found. Install Go 1.25+ and add it to PATH."
    exit 1
}

# Load .env for BAIZE_API_KEY etc. when not already set in the environment.
$envFile = Join-Path $Root ".env"
if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        if ($_ -match '^\s*#' -or $_ -notmatch '^\s*([^#=]+)=(.*)$') { return }
        $name = $matches[1].Trim()
        $value = $matches[2].Trim().Trim('"').Trim("'")
        if ($name -and -not [string]::IsNullOrWhiteSpace($value) -and -not (Get-Item "Env:$name" -ErrorAction SilentlyContinue)) {
            Set-Item -Path "Env:$name" -Value $value
        }
    }
}

# Chat UI is go:embed'd from internal/ui/dist. Rebuild when sources drift so
# locale/strings/UI fixes actually show up after .\serve.cmd.
if ($skipUI) {
    Write-Host "skipping chat UI build (-SkipUI / BAIZE_SKIP_UI=1)"
} else {
    Ensure-ChatUI -Force:$forceUI
}

$serveArgs = @("serve")
if ($baizeArgs.Count -gt 0) {
    $serveArgs += $baizeArgs
}
# 用 go build 编译后直接运行，而不是 go run：
# Windows 下 go run 会额外包一层子进程，Ctrl+C 无法可靠地把信号传给
# baize 进程，导致优雅退出失效、临时 exe 残留。
$exe = Join-Path $Root "bin\baize.exe"
$binDir = Split-Path -Parent $exe
if (-not (Test-Path -LiteralPath $binDir)) {
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
}
Write-Host "go build -o bin\baize.exe ./cmd/baize  (cwd=$Root)"
& go build -o $exe ./cmd/baize
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "baize $serveArgs"
& $exe @serveArgs
exit $LASTEXITCODE
