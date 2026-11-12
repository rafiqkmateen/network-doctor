; Network Doctor installer for Windows. Built with Inno Setup 6.
; Purpose: install the program, and only the optional parts the person picks.
; Owner: Rafiq. Version 0.1.0. Change list 11; requirements ND-110 to ND-120.
; Nothing in this script reaches the internet.

#define AppName "Network Doctor"
#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif
#ifndef StageDir
  #define StageDir "..\dist\installer-stage"
#endif

[Setup]
AppId={{A961DEB3-6983-4BD8-918C-C55D39EDE484}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=Rafiq
DefaultDirName={autopf}\Network Doctor
DisableDirPage=yes
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayName={#AppName}
UninstallDisplayIcon={app}\netdoctor.exe
OutputDir=..\dist\release
OutputBaseFilename=NetworkDoctor-Setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
WizardSizePercent=130,130
; With no /COMPONENTS switch a silent install gets the first type below: the on-demand check only.

[Types]
Name: "reactive"; Description: "On-demand check only"
Name: "full"; Description: "Everything: the on-demand check, the weekly check and alerts"
Name: "custom"; Description: "Choose the parts myself"; Flags: iscustom

[Components]
Name: "core"; Description: "Check on demand (always included). Run a full check any time you want one. Nothing leaves your computer."; Types: reactive full custom; Flags: fixed
Name: "scheduled"; Description: "Weekly check. Once a week, Monday at 0900, saves your speed to a file. Nothing leaves your computer."; Types: full
Name: "proactive"; Description: "Alerts. A small helper starts when you log on and runs all the time. It tells you on screen when your connection drops, stays slow, or a new device appears. Nothing leaves your computer."; Types: full

[Dirs]
Name: "{commonappdata}\NetworkDoctor"; Permissions: users-modify

[Files]
Source: "{#StageDir}\netdoctor.exe"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "{#StageDir}\netdoctorw.exe"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "{#StageDir}\GUIDE.md"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "installed.marker"; DestDir: "{app}"; Components: core; Flags: ignoreversion

[Run]
Filename: "schtasks.exe"; Parameters: "/Create /TN ""Network Doctor weekly check"" /TR ""\""{app}\netdoctorw.exe\"" weekly"" /SC WEEKLY /D MON /ST 09:00 /RU ""BUILTIN\Users"" /F"; Flags: runhidden; Components: scheduled; StatusMsg: "Setting up the weekly check..."
Filename: "schtasks.exe"; Parameters: "/Create /TN ""Network Doctor alerts"" /TR ""\""{app}\netdoctorw.exe\"" watch"" /SC ONLOGON /RU ""BUILTIN\Users"" /F"; Flags: runhidden; Components: proactive; StatusMsg: "Setting up alerts..."

[UninstallRun]
Filename: "schtasks.exe"; Parameters: "/Delete /TN ""Network Doctor weekly check"" /F"; Flags: runhidden; RunOnceId: "DelWeekly"
Filename: "schtasks.exe"; Parameters: "/Delete /TN ""Network Doctor alerts"" /F"; Flags: runhidden; RunOnceId: "DelAlerts"
Filename: "taskkill.exe"; Parameters: "/IM netdoctorw.exe /F"; Flags: runhidden; RunOnceId: "StopHelper"

[Code]
// Uninstall: ask once whether to keep the saved results. Yes (keep) is the default, so a
// silent uninstall keeps them. IT can add /DELETERESULTS=1 to delete them without asking.
var
  DeleteResults: Boolean;

function InitializeUninstall(): Boolean;
begin
  if ExpandConstant('{param:DELETERESULTS|0}') = '1' then
    DeleteResults := True
  else if UninstallSilent then
    DeleteResults := False  // silent means nobody can answer: keep the results
  else
    DeleteResults := MsgBox('Keep your saved results?' + #13#10#13#10 +
      'Yes keeps the weekly spreadsheet, the alert log and your list of known devices.' + #13#10 +
      'No deletes them too.', mbConfirmation, MB_YESNO or MB_DEFBUTTON1) = IDNO;
  Result := True;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usPostUninstall) and DeleteResults then
    DelTree(ExpandConstant('{commonappdata}\NetworkDoctor'), True, True, True);
end;
