# Builds every system, zips each with the one-page guide, and writes checksums.
# Purpose: make the files you take to another computer. Owner: Rafiq. Version 0.1.0.
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$out = 'dist\release'
Remove-Item $out -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $out | Out-Null
foreach ($t in 'windows/amd64','linux/amd64','linux/arm64','darwin/amd64','darwin/arm64') {
  $os,$arch = $t.Split('/')
  $ext = if ($os -eq 'windows') { '.exe' } else { '' }
  $stage = "dist\stage-$os-$arch"
  New-Item -ItemType Directory -Force $stage | Out-Null
  $env:GOOS = $os; $env:GOARCH = $arch; $env:CGO_ENABLED = '0'
  go build -trimpath -ldflags '-s -w' -o "$stage\netdoctor$ext" .\cmd\netdoctor
  Copy-Item docs\GUIDE.md $stage\GUIDE.md
  Compress-Archive -Path "$stage\*" -DestinationPath "$out\netdoctor-$os-$arch.zip" -Force
  Remove-Item $stage -Recurse -Force
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Get-ChildItem $out -Filter *.zip | ForEach-Object { "$((Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower())  $($_.Name)" } | Set-Content "$out\SHA256SUMS.txt"
Get-ChildItem $out | Select-Object Name, Length
