# Installs and uninstalls the setup file for each of the four choices and checks the result.
# Purpose: prove requirements ND-110 to ND-120 on a clean Windows computer. Owner: Rafiq.
# Version 0.3.1 (0.3.1, 2026-11-23: the planted notes file cannot be read, since a changed setting is now settled at
# removal and its note dropped; 0.3.0, 2026-11-23: bare /KEEPRESULTS and /KEEPRESULTS=yes keep the results; before the plain removal an
# undo note for a fix never applied is planted, so the uninstaller's undo step must run, change nothing and keep only
# the undo notes (ND-118, ND-316); 0.2.0, 2026-11-23: removal deletes the results by default; /KEEPRESULTS=1 keeps them, also with the
# old /DELETERESULTS=1, which alone only repeats the default (ND-118); 0.1.0: first version).
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

# Un: the removal switches given; Keep: whether the results folder must still be there afterwards;
# Plant: an undo note for a fix never applied is put in place first (ND-316).
$cases = @(
  @{ Name = 'neither';   Parts = '';                    Sched = $false; Pro = $false; Un = @('/KEEPRESULTS');                        Keep = $true;  Plant = $false },
  @{ Name = 'scheduled'; Parts = 'scheduled';           Sched = $true;  Pro = $false; Un = @();                                      Keep = $false; Plant = $true  },
  @{ Name = 'proactive'; Parts = 'proactive';           Sched = $false; Pro = $true;  Un = @('/KEEPRESULTS=yes', '/DELETERESULTS=1'); Keep = $true;  Plant = $false },
  @{ Name = 'both';      Parts = 'scheduled,proactive'; Sched = $true;  Pro = $true;  Un = @('/DELETERESULTS=1');                    Keep = $false; Plant = $false }
)
# The planted notes file cannot be read, so the undo changes nothing and must count it as not put back. (A note for a
# setting changed by hand since would now be settled and deleted, not kept, so it would not prove the kept notes.)
$planted = 'not notes'
$inet = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
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
  # ND-289: the settings schtasks cannot give, set by the installer's own step after each task.
  if ($c.Sched) {
    $ws = (Get-ScheduledTask -TaskName $weekly -ErrorAction SilentlyContinue).Settings
    Check ($ws.StartWhenAvailable -eq $true) 'weekly task runs as soon as it can after a missed start'
    Check ($ws.ExecutionTimeLimit -eq 'PT0S') 'weekly task has no run-time limit'
    Check (($ws.DisallowStartIfOnBatteries -eq $false) -and ($ws.StopIfGoingOnBatteries -eq $false)) 'weekly task runs on battery'
  }
  if ($c.Pro) {
    $at = Get-ScheduledTask -TaskName $alerts -ErrorAction SilentlyContinue
    $as = $at.Settings
    Check (($as.RestartCount -eq 3) -and ($as.RestartInterval -eq 'PT1M')) 'alerts task is started again if it fails (every minute, 3 times)'
    Check ($as.ExecutionTimeLimit -eq 'PT0S') 'alerts task has no run-time limit'
    Check (($as.DisallowStartIfOnBatteries -eq $false) -and ($as.StopIfGoingOnBatteries -eq $false)) 'alerts task runs on battery'
    Check ($as.MultipleInstances -eq 'IgnoreNew') 'alerts task never starts a second copy'
    Check ($at.Triggers[0].Repetition.Interval -eq 'PT5M') 'alerts task comes back within 5 minutes whatever stopped it'
  }
  Check (Entry) 'entry in Apps and features'
  Check (Test-Path "$app\unins000.exe") 'uninstaller is in the program folder'
  $v = & "$app\netdoctor.exe" version
  Check ($LASTEXITCODE -eq 0) "program runs ($v)"
  & "$app\netdoctor.exe" known add 00:11:22:33:44:55 test | Out-Null
  Check (Test-Path "$data\known-devices.txt") 'results are saved in ProgramData'
  if ($c.Plant) {
    [IO.File]::WriteAllText("$data\fix-saved.json", $planted)
    $proxyBefore = (Get-ItemProperty $inet -ErrorAction SilentlyContinue | Select-Object ProxyEnable, ProxyServer | ConvertTo-Json -Compress)
  }
  $un = @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') + $c.Un
  $un += "/LOG=$env:TEMP\ndunins-$($c.Name).log"
  $code = RunWait "$app\unins000.exe" $un "uninstall ($($c.Name))"
  Check ($code -eq 0) 'uninstall finished'
  # The first-stage uninstaller can hand over to a temporary copy; give it a moment to finish.
  for ($i = 0; $i -lt 30 -and (Test-Path "$app\unins000.exe"); $i++) { Start-Sleep -Seconds 2 }
  Check (-not (Test-Path "$app\netdoctor.exe")) 'program is gone'
  Check ((-not (TaskExists $weekly)) -and (-not (TaskExists $alerts))) 'both tasks are gone'
  Check (-not (Entry)) 'Apps and features entry is gone'
  $how = if ($c.Un.Count) { $c.Un -join ' ' } else { 'no switch' }
  if ($c.Keep) { Check (Test-Path "$data\known-devices.txt") "saved results kept with $how"; Remove-Item $data -Recurse -Force }
  elseif ($c.Plant) {
    $left = @(Get-ChildItem $data -Force -ErrorAction SilentlyContinue | ForEach-Object Name)
    $other = @($left | Where-Object { $_ -notin 'fix-saved.json', 'fix-log.txt', 'fix-enabled-here.txt', 'fix-not-undone.txt' })
    Check (($left -contains 'fix-saved.json') -and ($other.Count -eq 0)) "with a fix not put back only the undo notes are left (left: $($left -join ', '))"
    $note = Get-Content "$data\fix-not-undone.txt" -Raw -ErrorAction SilentlyContinue
    Check ($note -match 'could not put back') "the note says the change could not be put back: $note"
    $proxyAfter = (Get-ItemProperty $inet -ErrorAction SilentlyContinue | Select-Object ProxyEnable, ProxyServer | ConvertTo-Json -Compress)
    Check ($proxyAfter -eq $proxyBefore) 'the proxy setting was not changed'
    Get-Content "$env:TEMP\ndunins-$($c.Name).log" -ErrorAction SilentlyContinue | Select-String 'put back' | ForEach-Object { Write-Host "        log: $_" }
    Remove-Item $data -Recurse -Force -ErrorAction SilentlyContinue   # the planted note is the test's own
  }
  else { Check (-not (Test-Path $data)) "saved results and settings deleted with $how" }
}
if ($script:fails -gt 0) { Write-Host "$script:fails check(s) failed"; exit 1 }
Write-Host 'All installer checks passed.'
exit 0   # a task lookup that was expected to fail leaves a non-zero code behind; the checks above already decided
