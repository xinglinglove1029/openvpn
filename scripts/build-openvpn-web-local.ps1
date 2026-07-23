$ErrorActionPreference = 'Stop'

$repo = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $repo 'dist'
$work = Join-Path $dist '.work'
$package = './cmd/openvpn-web'

function Assert-RepoChildPath {
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [Parameter(Mandatory = $true)][string]$Root
  )

  $fullPath = [System.IO.Path]::GetFullPath($Path)
  $fullRoot = [System.IO.Path]::GetFullPath($Root).TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
  $separator = [System.IO.Path]::DirectorySeparatorChar

  if (-not ($fullPath.Equals($fullRoot, [System.StringComparison]::OrdinalIgnoreCase) -or $fullPath.StartsWith("$fullRoot$separator", [System.StringComparison]::OrdinalIgnoreCase))) {
    throw "Refuse to operate outside repository: $fullPath"
  }

  return $fullPath
}

$dist = Assert-RepoChildPath -Path $dist -Root $repo
$work = Assert-RepoChildPath -Path $work -Root $repo

if (Test-Path -LiteralPath $dist) {
  Remove-Item -LiteralPath $dist -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $dist | Out-Null
New-Item -ItemType Directory -Force -Path $work | Out-Null

$commit = $null
try {
  $commit = git -C $repo rev-parse --short HEAD 2>$null
} catch {
  $commit = $null
}
if ($LASTEXITCODE -ne 0 -or -not $commit) {
  $commit = 'unknown'
}

$date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$version = 'dev'

$targets = @(
  @{ GOOS = 'linux'; GOARCH = 'amd64'; Name = 'openvpn-web-linux-x86_64'; Binary = 'openvpn-web'; Archive = 'openvpn-web-linux-x86_64.tar.gz'; Format = 'tar.gz' },
  @{ GOOS = 'linux'; GOARCH = 'arm64'; Name = 'openvpn-web-linux-aarch64'; Binary = 'openvpn-web'; Archive = 'openvpn-web-linux-aarch64.tar.gz'; Format = 'tar.gz' },
  @{ GOOS = 'linux'; GOARCH = 'arm'; GOARM = '6'; Name = 'openvpn-web-linux-armv6l'; Binary = 'openvpn-web'; Archive = 'openvpn-web-linux-armv6l.tar.gz'; Format = 'tar.gz' },
  @{ GOOS = 'linux'; GOARCH = 'arm'; GOARM = '7'; Name = 'openvpn-web-linux-armv7l'; Binary = 'openvpn-web'; Archive = 'openvpn-web-linux-armv7l.tar.gz'; Format = 'tar.gz' },
  @{ GOOS = 'windows'; GOARCH = 'amd64'; Name = 'openvpn-web-windows-x86_64'; Binary = 'openvpn-web.exe'; Archive = 'openvpn-web-windows-x86_64.zip'; Format = 'zip' },
  @{ GOOS = 'darwin'; GOARCH = 'amd64'; Name = 'openvpn-web-darwin-x86_64'; Binary = 'openvpn-web'; Archive = 'openvpn-web-darwin-x86_64.tar.gz'; Format = 'tar.gz' },
  @{ GOOS = 'darwin'; GOARCH = 'arm64'; Name = 'openvpn-web-darwin-aarch64'; Binary = 'openvpn-web'; Archive = 'openvpn-web-darwin-aarch64.tar.gz'; Format = 'tar.gz' }
)

$runtimeReadme = @'
openvpn-web release artifact

This archive contains the openvpn-web management server binary.
It is not an OpenVPN desktop client.

Recommended production deployment:
- Use the Docker image xinglinglove1029/openvpn for the full OpenVPN + openvpn-web runtime.

Generic binary usage:
- Linux/macOS: chmod +x ./openvpn-web && OVPN_DATA=/path/to/data ./openvpn-web
- Windows: .\openvpn-web.exe only starts the web server part and cannot provide the full Linux OpenVPN runtime.
'@

Push-Location $repo
try {
  Push-Location (Join-Path $repo 'frontend\admin')
  try {
    $npm = 'npm'
    if (Get-Command npm.cmd -ErrorAction SilentlyContinue) {
      $npm = 'npm.cmd'
    }

    Write-Host '==> building React admin assets'
    & $npm run build
  } finally {
    Pop-Location
  }

  foreach ($target in $targets) {
    $env:CGO_ENABLED = '0'
    $env:GOOS = $target.GOOS
    $env:GOARCH = $target.GOARCH

    if ($target.ContainsKey('GOARM')) {
      $env:GOARM = $target.GOARM
      $label = "$($target.GOOS)/$($target.GOARCH)/arm$($target.GOARM)"
    } else {
      Remove-Item Env:GOARM -ErrorAction SilentlyContinue
      $label = "$($target.GOOS)/$($target.GOARCH)"
    }

    $stage = Join-Path $work $target.Name
    New-Item -ItemType Directory -Force -Path $stage | Out-Null

    $output = Join-Path $stage $target.Binary
    Write-Host "==> building $label -> $($target.Name)"
    go build -trimpath -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.date=$date -X main.builtBy=manual" -o $output $package

    Set-Content -Encoding UTF8 -Path (Join-Path $stage 'README-runtime.txt') -Value $runtimeReadme
    if (Test-Path -LiteralPath (Join-Path $repo 'LICENSE')) {
      Copy-Item -LiteralPath (Join-Path $repo 'LICENSE') -Destination (Join-Path $stage 'LICENSE') -Force
    }

    $archive = Join-Path $dist $target.Archive
    Write-Host "==> packaging $($target.Archive)"
    if ($target.Format -eq 'zip') {
      Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -Force
    } else {
      $items = Get-ChildItem -LiteralPath $stage -Name
      & tar -C $stage -czf $archive @items
    }
  }

  if (Test-Path -LiteralPath $work) {
    Remove-Item -LiteralPath $work -Recurse -Force
  }

  Get-ChildItem -LiteralPath $dist -File |
    Sort-Object Name |
    ForEach-Object {
      $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
      "$hash  $($_.Name)"
    } |
    Set-Content -Encoding ASCII -Path (Join-Path $dist 'openvpn-web_sha256_checksums.txt')
} finally {
  Remove-Item Env:GOOS -ErrorAction SilentlyContinue
  Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
  Remove-Item Env:GOARM -ErrorAction SilentlyContinue
  Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Pop-Location
}
