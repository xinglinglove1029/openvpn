$ErrorActionPreference = 'Stop'

$repo = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

function Remove-WorkspacePath {
  param([string] $RelativePath, [switch] $Recurse)

  $target = Join-Path $repo $RelativePath
  if (-not (Test-Path -LiteralPath $target)) {
    return
  }

  $resolved = (Resolve-Path -LiteralPath $target).Path
  if (-not $resolved.StartsWith($repo, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "Refuse to remove path outside workspace: $resolved"
  }

  if ($Recurse) {
    Remove-Item -LiteralPath $resolved -Recurse -Force
  } else {
    Remove-Item -LiteralPath $resolved -Force
  }
}

# The legacy frontend has been removed. The active admin UI is the React/Vite
# project under frontend, built into
# internal/openvpnweb/templates/static/admin.
Remove-WorkspacePath 'src' -Recurse
Remove-WorkspacePath 'build\openvpn-web'
Remove-WorkspacePath 'build\openvpn-web.exe'

Write-Host 'Legacy frontend/runtime artifacts cleaned.'
