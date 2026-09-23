# Windows launcher: loads .env (if present), then runs `baize serve` with the
# default configs/config.yaml. Pass -config to use another file, e.g.
#   .\serve.cmd -config configs\config.local.yaml
$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $Root

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

$sdkGo = Join-Path $env:USERPROFILE "sdk\go\bin"
if (Test-Path (Join-Path $sdkGo "go.exe")) {
    $env:Path = "$sdkGo;" + $env:Path
}
if (-not $env:GOPROXY) {
    $env:GOPROXY = "https://goproxy.cn,direct"
}
if (-not $env:GOSUMDB) {
    $env:GOSUMDB = "sum.golang.google.cn"
}

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    Write-Error "go not found. Install Go 1.22+ or put it on PATH (e.g. %USERPROFILE%\sdk\go\bin)."
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

$serveArgs = @("serve")
if ($args.Count -gt 0) {
    $serveArgs += $args
}
# 用 go build 编译后直接运行，而不是 go run：
# Windows 下 go run 会额外包一层子进程，Ctrl+C 无法可靠地把信号传给
# baize 进程，导致优雅退出失效、临时 exe 残留。
$exe = Join-Path $Root "bin\baize.exe"
Write-Host "go build -o bin\baize.exe ./cmd/baize  (cwd=$Root)"
& go build -o $exe ./cmd/baize
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "baize $serveArgs"
& $exe @serveArgs
exit $LASTEXITCODE
