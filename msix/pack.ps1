# Packs the built launcher into MSIX packages for the Microsoft Store (one per architecture).
# The identity values below are the ones Partner Center shows under Product identity (public; they end up inside every package).
# usage: pack.ps1 -Version 1.9.1 -In dist -Out dist/msix
param([string]$Version, [string]$In = "dist", [string]$Out = "dist/msix")
$ErrorActionPreference = "Stop"
$name = if ($env:MSIX_IDENTITY_NAME) { $env:MSIX_IDENTITY_NAME } else { "blham.CGMSLauncher" }
$publisher = if ($env:MSIX_PUBLISHER) { $env:MSIX_PUBLISHER } else { "CN=BF43D319-CFFF-4A2B-8B00-2FD7911C65BD" }
$pubDisplay = if ($env:MSIX_PUBLISHER_DISPLAY) { $env:MSIX_PUBLISHER_DISPLAY } else { "blham" }
$display = if ($env:MSIX_DISPLAY_NAME) { $env:MSIX_DISPLAY_NAME } else { "CGMS Launcher" }
$v = ($Version -replace '[^0-9.]', '').Trim('.')
$parts = @($v.Split('.') | Where-Object { $_ -ne '' })
while ($parts.Count -lt 3) { $parts += '0' }
$v4 = ($parts[0..2] -join '.') + '.0'          # the Store requires the fourth part to be 0
$makeappx = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\makeappx.exe" | Sort-Object FullName | Select-Object -Last 1
if (-not $makeappx) { throw "makeappx.exe not found (Windows SDK)" }
New-Item -ItemType Directory -Force $Out | Out-Null
foreach ($a in @(@{exe = "AsiaWarLauncher-windows-x64.exe"; arch = "x64" }, @{exe = "AsiaWarLauncher-windows-arm64.exe"; arch = "arm64" })) {
  $lay = Join-Path $Out ("layout-" + $a.arch)
  if (Test-Path $lay) { Remove-Item -Recurse -Force $lay }
  New-Item -ItemType Directory -Force (Join-Path $lay "Assets") | Out-Null
  Copy-Item (Join-Path $In $a.exe) (Join-Path $lay "AsiaWarLauncher.exe")
  Copy-Item "msix/Assets/*" (Join-Path $lay "Assets")
  $m = Get-Content "msix/AppxManifest.xml" -Raw -Encoding UTF8
  $m = $m.Replace("@NAME@", $name).Replace("@PUBLISHER@", $publisher).Replace("@PUBLISHER_DISPLAY@", $pubDisplay).Replace("@DISPLAY_NAME@", $display).Replace("@VERSION@", $v4).Replace("@ARCH@", $a.arch)
  [IO.File]::WriteAllText((Join-Path $lay "AppxManifest.xml"), $m, (New-Object Text.UTF8Encoding($false)))
  $pkg = Join-Path $Out ("CGMSLauncher-" + $v4 + "-" + $a.arch + ".msix")
  & $makeappx.FullName pack /o /d $lay /p $pkg
  if ($LASTEXITCODE -ne 0) { throw "makeappx failed for $($a.arch)" }
  Remove-Item -Recurse -Force $lay
}
Get-ChildItem $Out
