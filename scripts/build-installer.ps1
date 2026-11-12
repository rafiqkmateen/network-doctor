# Builds the Windows installer (setup.exe) with Inno Setup, then zips it with the guide.
# Purpose: make the file a customer or IT runs. Owner: Rafiq. Version 0.2.0. Change list 11 and 19.
# Needs Inno Setup 6 (GitHub's Windows machines have it; on a laptop install it first).
# -WindowlessTag: Smart App Control judges each new file on its own. A different build id makes a different
# netdoctorw.exe, so a laptop that blocked one build can try another. Release builds leave it empty.
param([string]$Version = '0.1.0', [string]$WindowlessTag = '')
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$stage = 'dist\installer-stage'
Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $stage | Out-Null
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
# netdoctor.exe shows a console for people who type commands. netdoctorw.exe has no console
# window; the weekly and alert tasks use it so nothing flashes on screen.
go build -trimpath -ldflags '-s -w' -o "$stage\netdoctor.exe" .\cmd\netdoctor
$wflags = '-s -w -H=windowsgui'
if ($WindowlessTag) { $wflags += " -buildid=$WindowlessTag" }
go build -trimpath -ldflags $wflags -o "$stage\netdoctorw.exe" .\cmd\netdoctor
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Copy-Item docs\GUIDE.md "$stage\GUIDE.md"
$iscc = @("${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe", "$env:ProgramFiles\Inno Setup 6\ISCC.exe", "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe") | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $iscc) { throw 'Inno Setup 6 is not installed.' }
& $iscc "/DAppVersion=$Version" installer\netdoctor.iss
if ($LASTEXITCODE -ne 0) { throw "Inno Setup failed with code $LASTEXITCODE" }
$setup = "dist\release\NetworkDoctor-Setup-$Version.exe"
Compress-Archive -Path $setup, docs\GUIDE.md -DestinationPath "dist\release\NetworkDoctor-Setup-$Version.zip" -Force
Get-ChildItem dist\release | Select-Object Name, Length
