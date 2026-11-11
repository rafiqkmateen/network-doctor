package collect

// These PowerShell snippets are the only commands the Windows collector runs.
// A test checks that every one of them only reads.

const scriptAdapter = `
$ErrorActionPreference = 'SilentlyContinue'
$r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1
$a = $null
if ($r) { $a = Get-NetAdapter -InterfaceIndex $r.InterfaceIndex }
if (-not $a) { $a = Get-NetAdapter -Physical | Where-Object { $_.Status -eq 'Up' } | Select-Object -First 1 }
if (-not $a) { $a = Get-NetAdapter -Physical | Select-Object -First 1 }
$st = $null
if ($a) { $st = Get-NetAdapterStatistics -Name $a.Name }
$ip = $null
if ($a) { $ip = Get-NetIPAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 | Select-Object -First 1 }
$dd = ''
if ($a -and $a.DriverDate) { $dd = $a.DriverDate.ToString('yyyy-MM-dd') }
$pm = ''
if ($a) { $p = Get-NetAdapterPowerManagement -Name $a.Name; if ($p) { $pm = [string]$p.AllowComputerToTurnOffDevice } }
$hint = ''
if ($a) {
  $adv = @(Get-NetAdapterAdvancedProperty -Name $a.Name | Where-Object { $_.DisplayName -match 'Energy|Green|Power Saving|EEE|Low Power' })
  if ($adv.Count -gt 0) {
    $hint = 'Disabled'
    foreach ($x in $adv) { if ([string]$x.DisplayValue -match '^(Enabled|On|Auto|Max|Maximum)') { $hint = 'Enabled' } }
  }
}
[pscustomobject]@{
  Name = [string]$a.Name; Desc = [string]$a.InterfaceDescription; Status = [string]$a.Status
  Media = [string]$a.PhysicalMediaType; LinkBps = [int64]$a.Speed; FullDuplex = [bool]$a.FullDuplex
  Gateway = [string]$r.NextHop; IfIndex = [int]$a.ifIndex
  RxErr = [int64]$st.ReceivedPacketErrors; TxErr = [int64]$st.OutboundPacketErrors
  DriverVersion = [string]$a.DriverVersion; DriverDate = $dd; PowerSaving = $pm; PowerHint = $hint
  IP = [string]$ip.IPAddress; Prefix = [int]$ip.PrefixLength
} | ConvertTo-Json -Compress
`

const scriptEvents = `
$ErrorActionPreference = 'SilentlyContinue'
$e = Get-WinEvent -FilterHashtable @{LogName='Microsoft-Windows-NetworkProfile/Operational'; Id=10000,10001; StartTime=(Get-Date).AddDays(-%DAYS%)}
$o = @($e | ForEach-Object { [pscustomobject]@{ T = $_.TimeCreated.ToUniversalTime().ToString('o'); Id = $_.Id } })
ConvertTo-Json -InputObject $o -Compress
`

const scriptThroughput = `
$ErrorActionPreference = 'SilentlyContinue'
function Tot { $s = Get-NetAdapter -Physical | Where-Object { $_.Status -eq 'Up' } | Get-NetAdapterStatistics; $rx = 0; $tx = 0; foreach ($x in $s) { $rx += $x.ReceivedBytes; $tx += $x.SentBytes }; return @($rx, $tx) }
$a = Tot
[System.Threading.Thread]::Sleep(2000)
$b = Tot
$names = @{}
Get-Process | ForEach-Object { $names[[int]$_.Id] = $_.ProcessName }
$procs = @(Get-NetTCPConnection -State Established | Group-Object OwningProcess | ForEach-Object { $n = $names[[int]$_.Name]; if ($n) { [pscustomobject]@{ Name = $n; Conns = [int]$_.Count } } })
[pscustomobject]@{ Rx = [double]($b[0] - $a[0]) / 2; Tx = [double]($b[1] - $a[1]) / 2; Procs = $procs } | ConvertTo-Json -Compress -Depth 4
`

const scriptNeighbors = `
$ErrorActionPreference = 'SilentlyContinue'
$n = Get-NetNeighbor -AddressFamily IPv4 | Where-Object { $_.State -ne 'Unreachable' -and $_.LinkLayerAddress -and $_.LinkLayerAddress -ne 'FF-FF-FF-FF-FF-FF' -and $_.LinkLayerAddress -ne '00-00-00-00-00-00' -and $_.LinkLayerAddress -notlike '01-00-5E*' }
$o = @($n | ForEach-Object { [pscustomobject]@{ IP = [string]$_.IPAddress; MAC = [string]$_.LinkLayerAddress } })
ConvertTo-Json -InputObject $o -Compress
`

const scriptSettings = `
$ErrorActionPreference = 'SilentlyContinue'
$r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1
$dns = @()
if ($r) { $dns = @(Get-DnsClientServerAddress -InterfaceIndex $r.InterfaceIndex -AddressFamily IPv4 | Select-Object -ExpandProperty ServerAddresses) }
$vpn = @(Get-VpnConnection | Where-Object { $_.ConnectionStatus -eq 'Connected' }).Count
$tun = @(Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and ($_.InterfaceDescription -match 'VPN|TAP-|WireGuard|Wintun|Lightway|OpenVPN|Tunnel') }).Count
$px = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
$proxyOn = ($px.ProxyEnable -eq 1)
$bad = @(Get-CimInstance Win32_NetworkAdapter | Where-Object { $_.PhysicalAdapter -eq $true -and $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne $null } | ForEach-Object { $_.Name + ' (code ' + $_.ConfigManagerErrorCode + ')' })
[pscustomobject]@{ DNS = $dns; Vpn = ($vpn + $tun); Proxy = $proxyOn; Errors = $bad } | ConvertTo-Json -Compress -Depth 4
`
