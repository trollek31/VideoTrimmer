$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$work = Join-Path $root '.cache'
$dist = Join-Path $root 'dist'
New-Item -ItemType Directory -Force $work,$dist | Out-Null

# The end-user package contains the complete FFmpeg runtime archive. It is unpacked
# automatically by VideoTrimmer at first launch, so the user installs nothing.
# Prefer a locally supplied archive (for example the ffmpeg-9.0.1-full_build.zip
# uploaded by the developer); otherwise download the current x64 GPL shared build.
$runtimeZip = Join-Path $work 'ffmpeg-runtime.zip'
$runtimeUrl = 'https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-n9.0-latest-win64-gpl-shared-9.0.zip'

if (-not (Test-Path $runtimeZip)) {
    Write-Host 'Downloading FFmpeg Windows runtime...'
    Invoke-WebRequest -Uri $runtimeUrl -OutFile $runtimeZip
}
$sizeMB = [math]::Round((Get-Item $runtimeZip).Length / 1MB, 1)
if ($sizeMB -lt 40) { throw "FFmpeg runtime archive looks invalid or too small: $sizeMB MiB" }

$env:GOOS='windows'
$env:GOARCH='amd64'
$env:CGO_ENABLED='0'
$exe = Join-Path $dist 'VideoTrimmer.exe'
Push-Location $root
try {
    go build -trimpath -ldflags='-H=windowsgui -s -w' -o $exe .\cmd\videotrimmer
} finally { Pop-Location }

$pkg = Join-Path $dist 'VideoTrimmer_Windows'
Remove-Item $pkg -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force (Join-Path $pkg 'runtime') | Out-Null
Copy-Item $exe (Join-Path $pkg 'VideoTrimmer.exe') -Force
Copy-Item $runtimeZip (Join-Path $pkg 'runtime\ffmpeg-runtime.zip') -Force
Copy-Item (Join-Path $root 'README.md') (Join-Path $pkg 'README.md') -Force
Copy-Item (Join-Path $root 'BUILD_STATUS.md') (Join-Path $pkg 'BUILD_STATUS.md') -Force

@'
Video Trimmer — Portable Windows build

The complete FFmpeg Windows runtime is shipped inside runtime\ffmpeg-runtime.zip.
The application extracts the runtime automatically to a temporary directory when
needed. The end user does not need to install FFmpeg, Python, Go, .NET Runtime,
Visual C++ Redistributable, Node.js, or Java.

Third-party component: FFmpeg
https://ffmpeg.org/
The exact license/NOTICE files for the selected runtime must remain with a public release.
'@ | Set-Content (Join-Path $pkg 'THIRD_PARTY_NOTICES.txt') -Encoding UTF8

# Use Store compression for predictable speed. GitHub release hosting can distribute
# the resulting package directly; the inner FFmpeg archive remains compressed.
$zip = Join-Path $dist 'VideoTrimmer_Windows.zip'
Remove-Item $zip -Force -ErrorAction SilentlyContinue
Compress-Archive -Path (Join-Path $pkg '*') -DestinationPath $zip -CompressionLevel NoCompression
Write-Host "DONE: $zip"
