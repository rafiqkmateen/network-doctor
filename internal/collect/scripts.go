package collect

// These PowerShell snippets are the only commands the Windows collector runs.
// A test checks that every one of them only reads.

// scriptRoutes lists every IPv4 default route with its two metrics and the
// state of the adapter behind it. Go picks the route (see PickDefaultRoute).
const scriptRoutes = `
$ErrorActionPreference = 'SilentlyContinue'
$o = @(Get-NetRoute -DestinationPrefix '0.0.0.0/0' | ForEach-Object {
  $i = Get-NetIPInterface -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 | Select-Object -First 1
  $ad = Get-NetAdapter -InterfaceIndex $_.InterfaceIndex
  [pscustomobject]@{ IfIndex = [int]$_.InterfaceIndex; NextHop = [string]$_.NextHop; RouteMetric = [int]$_.RouteMetric; InterfaceMetric = [int]$i.InterfaceMetric; Status = [string]$ad.Status }
})
ConvertTo-Json -InputObject $o -Compress
`

// scriptAdapter reads the adapter. Go fills %IFINDEX% (an integer: the picked
// interface, 0 for none, -1 to choose the old way) and %GATEWAY% (a checked
// IPv4 address or empty). See adapterScript.
const scriptAdapter = `
$ErrorActionPreference = 'SilentlyContinue'
$ix = %IFINDEX%
$gw = '%GATEWAY%'
$a = $null
if ($ix -gt 0) {
  $a = Get-NetAdapter -InterfaceIndex $ix
  if (-not $a) { $gw = '' }
} elseif ($ix -lt 0) {
  $r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1
  if ($r) { $a = Get-NetAdapter -InterfaceIndex $r.InterfaceIndex; $gw = [string]$r.NextHop }
}
if (-not $a) { $a = Get-NetAdapter -Physical | Where-Object { $_.Status -eq 'Up' } | Select-Object -First 1 }
if (-not $a) { $a = Get-NetAdapter -Physical | Select-Object -First 1 }
$phys = @(Get-NetAdapter -Physical)
$wifiAd = @($phys | Where-Object { [string]$_.PhysicalMediaType -match '802\.11' -or [string]$_.InterfaceDescription -match 'wi-?fi|wireless' })
$wiredAd = @($phys | Where-Object { [string]$_.PhysicalMediaType -match '802\.3' -and $wifiAd -notcontains $_ })
$upAd = @($phys | Where-Object { $_.Status -eq 'Up' })
$st = $null
if ($a) { $st = Get-NetAdapterStatistics -Name $a.Name }
$ip = $null
if ($a) { $ip = Get-NetIPAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 | Sort-Object { ([string]$_.IPAddress).StartsWith('169.254.') } | Select-Object -First 1 }
$dd = ''
$inv = [Globalization.CultureInfo]::InvariantCulture
if ($a -and [int64]$a.DriverDateData -gt 0) { $dd = [DateTime]::FromFileTimeUtc([int64]$a.DriverDateData).ToString('yyyy-MM-dd', $inv) }
if ($a -and -not $dd -and $a.DriverDate) { $t = [datetime]::MinValue; if ([datetime]::TryParse([string]$a.DriverDate, $inv, [Globalization.DateTimeStyles]::None, [ref]$t)) { $dd = $t.ToString('yyyy-MM-dd', $inv) } }
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
  Gateway = $gw; IfIndex = [int]$a.ifIndex
  RxErr = [int64]$st.ReceivedPacketErrors; TxErr = [int64]$st.OutboundPacketErrors
  DriverVersion = [string]$a.DriverVersion; DriverDate = $dd; PowerSaving = $pm; PowerHint = $hint
  IP = [string]$ip.IPAddress; Prefix = [int]$ip.PrefixLength
  HasWired = ($wiredAd.Count -gt 0); HasWiFi = ($wifiAd.Count -gt 0); NoPhysicalUp = ($phys.Count -gt 0 -and $upAd.Count -eq 0)
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

// scriptSettings reads DNS, VPN, proxy and adapter errors. Go fills %IFINDEX%
// (an integer: the adapter the check read, or -1 to choose the old way).
// See settingsScript.
const scriptSettings = `
$ErrorActionPreference = 'SilentlyContinue'
$ix = %IFINDEX%
if ($ix -le 0) {
  $r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1
  $ix = 0
  if ($r) { $ix = [int]$r.InterfaceIndex }
}
$dns = @()
if ($ix -gt 0) { $dns = @(Get-DnsClientServerAddress -InterfaceIndex $ix -AddressFamily IPv4 | Select-Object -ExpandProperty ServerAddresses) }
$vpn = @(Get-VpnConnection | Where-Object { $_.ConnectionStatus -eq 'Connected' }).Count
$tun = @(Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and ($_.InterfaceDescription -match 'VPN|TAP-|WireGuard|Wintun|Lightway|OpenVPN|Tunnel') }).Count
$px = Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
$proxyOn = ($px.ProxyEnable -eq 1)
$bad = @(Get-CimInstance Win32_NetworkAdapter | Where-Object { $_.PhysicalAdapter -eq $true -and $_.ConfigManagerErrorCode -ne 0 -and $_.ConfigManagerErrorCode -ne $null } | ForEach-Object { $_.Name + ' (code ' + $_.ConfigManagerErrorCode + ')' })
[pscustomobject]@{ DNS = $dns; Vpn = ($vpn + $tun); Proxy = $proxyOn; Errors = $bad } | ConvertTo-Json -Compress -Depth 4
`
