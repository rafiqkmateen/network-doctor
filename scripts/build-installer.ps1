# Builds the Windows installer (setup.exe) with Inno Setup, then zips it with the guide.
# Purpose: make the file a customer or IT runs. Owner: Rafiq. Version 0.3.0.
# Needs Inno Setup 6 (GitHub's Windows machines have it; on your own computer install it first).
# -Version: left out, the version is read from internal/app/app.go, so the file name always follows the
# program (ND-211). Pass one only to build a different number on purpose.
# -WindowlessTag: Smart App Control judges each new file on its own. A different build id makes a different
# netdoctorw.exe, so a computer that blocked one build can try another. Release builds leave it empty.
param([string]$Version = '', [string]$WindowlessTag = '')
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
if (-not $Version) {
  $m = Select-String -Path 'internal/app/app.go' -Pattern 'const Version = "(\d+\.\d+\.\d+)"' | Select-Object -First 1
  if (-not $m) { throw 'Could not read the version from internal/app/app.go.' }
  $Version = $m.Matches[0].Groups[1].Value
}
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
