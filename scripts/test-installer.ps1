# Installs and uninstalls the setup file for each of the four choices and checks the result.
# Purpose: prove requirements ND-110 to ND-120 on a clean Windows computer. Owner: Rafiq. Version 0.1.0.
# Run it only on a test machine: it installs into Program Files and removes it again.
param([Parameter(Mandatory)][string]$Setup)
$ErrorActionPreference = 'Continue'
$app = Join-Path $env:ProgramFiles 'Network Doctor'
$data = Join-Path $env:ProgramData 'NetworkDoctor'
$weekly = 'Network Doctor weekly check'
$alerts = 'Network Doctor alerts'
$script:fails = 0

function Check($ok, $what) {
  if ($ok) { Write-Host "  PASS  $what" } else { Write-Host "  FAIL  $what"; $script:fails++ }
}
function TaskExists($name) { schtasks.exe /Query /TN $name *> $null; return ($LASTEXITCODE -eq 0) }
function Entry() { return [bool](Get-ChildItem 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall' -ErrorAction SilentlyContinue | Where-Object { ($_ | Get-ItemProperty).DisplayName -eq 'Network Doctor' }) }

# Start a program and wait up to two minutes. On a hang, show what was running and the log, then stop it.
function RunWait($file, $argList, $label) {
  $proc = Start-Process -FilePath $file -ArgumentList $argList -PassThru
  if (-not $proc.WaitForExit(120000)) {
    Write-Host "  HANG  $label did not finish in two minutes"
    Get-Process | Where-Object { $_.Name -match 'unins|_iu|_un|netdoctor|Setup' } | Format-Table Name, Id, Responding | Out-String | Write-Host
    $log = $argList | Where-Object { $_ -like '/LOG=*' } | ForEach-Object { $_.Substring(5).Trim('"') }
    if ($log -and (Test-Path $log)) { Get-Content $log -Tail 40 | Write-Host }
    $proc | Stop-Process -Force -ErrorAction SilentlyContinue
    $script:fails++
    return -1
  }
  return $proc.ExitCode
}

if ((Test-Path $app) -or (TaskExists $weekly) -or (TaskExists $alerts)) { throw 'This computer already has Network Doctor. Use a clean test machine.' }

$cases = @(
  @{ Name = 'neither';   Parts = '';                    Sched = $false; Pro = $false; Delete = $false },
  @{ Name = 'scheduled'; Parts = 'scheduled';           Sched = $true;  Pro = $false; Delete = $true  },
  @{ Name = 'proactive'; Parts = 'proactive';           Sched = $false; Pro = $true;  Delete = $false },
  @{ Name = 'both';      Parts = 'scheduled,proactive'; Sched = $true;  Pro = $true;  Delete = $true  }
)
foreach ($c in $cases) {
  Write-Host "== $($c.Name)"
  $args1 = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART')
  if ($c.Parts) { $args1 += "/COMPONENTS=$($c.Parts)" }
  $args1 += "/LOG=$env:TEMP\ndsetup-$($c.Name).log"
  $code = RunWait $Setup $args1 "install ($($c.Name))"
  Check ($code -eq 0) 'install finished'
  Check (Test-Path "$app\netdoctor.exe") 'on-demand check is installed (always)'
  Check ((Test-Path "$app\netdoctorw.exe") -and (Test-Path "$app\GUIDE.md")) 'quiet program and guide are installed'
  Check (Test-Path "$app\installed.marker") 'installed marker is present'
  Check ((TaskExists $weekly) -eq $c.Sched) "weekly task present = $($c.Sched)"
  Check ((TaskExists $alerts) -eq $c.Pro) "log-on alerts task present = $($c.Pro)"
  Check (Entry) 'entry in Apps and features'
  Check (Test-Path "$app\unins000.exe") 'uninstaller is in the program folder'
  $v = & "$app\netdoctor.exe" version
  Check ($LASTEXITCODE -eq 0) "program runs ($v)"
  & "$app\netdoctor.exe" known add 00:11:22:33:44:55 test | Out-Null
  Check (Test-Path "$data\known-devices.txt") 'results are saved in ProgramData'
  $un = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART')
  if ($c.Delete) { $un += '/DELETERESULTS=1' }
  $un += "/LOG=$env:TEMP\ndunins-$($c.Name).log"
  $code = RunWait "$app\unins000.exe" $un "uninstall ($($c.Name))"
  Check ($code -eq 0) 'uninstall finished'
  # The first-stage uninstaller can hand over to a temporary copy; give it a moment to finish.
  for ($i = 0; $i -lt 30 -and (Test-Path "$app\unins000.exe"); $i++) { Start-Sleep -Seconds 2 }
  Check (-not (Test-Path "$app\netdoctor.exe")) 'program is gone'
  Check ((-not (TaskExists $weekly)) -and (-not (TaskExists $alerts))) 'both tasks are gone'
  Check (-not (Entry)) 'Apps and features entry is gone'
  if ($c.Delete) { Check (-not (Test-Path $data)) 'saved results deleted when asked' }
  else { Check (Test-Path "$data\known-devices.txt") 'saved results kept by default'; Remove-Item $data -Recurse -Force }
}
if ($script:fails -gt 0) { Write-Host "$script:fails check(s) failed"; exit 1 }
Write-Host 'All installer checks passed.'
exit 0   # a task lookup that was expected to fail leaves a non-zero code behind; the checks above already decided
