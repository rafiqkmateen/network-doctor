; Network Doctor installer for Windows. Built with Inno Setup 6.
; Purpose: install the program, and only the optional parts the person picks. Run it again to repair: it puts back
;   what the person chose and removes what they did not. The uninstaller (Apps and features) removes it all.
; Owner: Rafiq. Version 0.2.0. Change list 11, 19 and 20; requirements ND-110 to ND-120 and ND-209 to ND-211.
; The only thing this script ever fetches from the internet is Ookla's own Speedtest tool, from Ookla, only when the
; person ticks that they want it and that they accept Ookla's license, and never in a silent install.

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
Filename: "schtasks.exe"; Parameters: "/Delete /TN ""Network Doctor weekly check"" /F"; Flags: runhidden; Components: not scheduled; StatusMsg: "Removing the weekly check you did not choose..."
Filename: "schtasks.exe"; Parameters: "/Delete /TN ""Network Doctor alerts"" /F"; Flags: runhidden; Components: not proactive; StatusMsg: "Removing alerts you did not choose..."

[UninstallDelete]
Type: files; Name: "{app}\choices.json"
Type: filesandordirs; Name: "{app}\ookla"

[UninstallRun]
Filename: "schtasks.exe"; Parameters: "/Delete /TN ""Network Doctor weekly check"" /F"; Flags: runhidden; RunOnceId: "DelWeekly"
Filename: "schtasks.exe"; Parameters: "/Delete /TN ""Network Doctor alerts"" /F"; Flags: runhidden; RunOnceId: "DelAlerts"
Filename: "taskkill.exe"; Parameters: "/IM netdoctorw.exe /F"; Flags: runhidden; RunOnceId: "StopHelper"

[Code]
var
  DeleteResults: Boolean;
  OoklaPage: TWizardPage;
  OoklaOffer, OoklaAgree: TNewCheckBox;
  OoklaDownload: TDownloadWizardPage;
  OoklaAgreed: Boolean;   // the person ticked both boxes (never true in a silent install)
  OoklaFetched: Boolean;  // the file arrived and matched the fixed hash
  OoklaReady: Boolean;    // the tool is unpacked and ran once

const
  OoklaZipUrl = 'https://install.speedtest.net/app/cli/ookla-speedtest-1.2.0-win64.zip';
  OoklaZipSha256 = '13e3d888b845d301a556419e31f14ab9bff57e3f06089ef2fd3bdc9ba6841efa';
  OoklaEulaUrl = 'https://www.speedtest.net/about/eula';
  OoklaTermsUrl = 'https://www.speedtest.net/about/terms';
  OoklaCliPageUrl = 'https://www.speedtest.net/apps/cli';

function BoolWord(B: Boolean): String;
begin
  if B then Result := 'true' else Result := 'false';
end;

procedure OpenLink(Url: String);
var
  Code: Integer;
begin
  ShellExec('open', Url, '', '', SW_SHOWNORMAL, ewNoWait, Code);
end;

procedure OpenEula(Sender: TObject);
begin
  OpenLink(OoklaEulaUrl);
end;

procedure OpenTerms(Sender: TObject);
begin
  OpenLink(OoklaTermsUrl);
end;

procedure OpenCliPage(Sender: TObject);
begin
  OpenLink(OoklaCliPageUrl);
end;

function AddLink(Parent: TWizardPage; Caption: String; Top: Integer; Handler: TNotifyEvent): TNewStaticText;
begin
  Result := TNewStaticText.Create(Parent);
  Result.Parent := Parent.Surface;
  Result.Caption := Caption;
  Result.Top := Top;
  Result.Font.Style := [fsUnderline];
  Result.Font.Color := clBlue;
  Result.Cursor := crHand;
  Result.OnClick := Handler;
end;

procedure InitializeWizard();
var
  Note: TNewStaticText;
  Y: Integer;
begin
  OoklaPage := CreateCustomPage(wpSelectComponents, 'Speed tool (optional)', 'Network Doctor can measure your speed with Ookla Speedtest. This is your choice.');
  Note := TNewStaticText.Create(OoklaPage);
  Note.Parent := OoklaPage.Surface;
  Note.WordWrap := True;
  Note.Width := OoklaPage.SurfaceWidth;
  Note.Caption := 'Ookla Speedtest is a free command-line tool from Ookla, the company behind speedtest.net. ' +
    'Ookla allows it for your own personal, non-commercial use on a computer you own. It is not part of Network Doctor, ' +
    'Network Doctor never passes it to anyone, and without it Network Doctor measures speed with its own, lighter test. ' +
    'Each Ookla test uses about two and a half gigabytes of data.';
  Note.AutoSize := True;
  Y := Note.Top + Note.Height + ScaleY(14);

  OoklaOffer := TNewCheckBox.Create(OoklaPage);
  OoklaOffer.Parent := OoklaPage.Surface;
  OoklaOffer.Caption := 'Download Ookla Speedtest now, straight from Ookla';
  OoklaOffer.Checked := False;
  OoklaOffer.Top := Y;
  OoklaOffer.Width := OoklaPage.SurfaceWidth;
  Y := Y + OoklaOffer.Height + ScaleY(10);

  OoklaAgree := TNewCheckBox.Create(OoklaPage);
  OoklaAgree.Parent := OoklaPage.Surface;
  OoklaAgree.Caption := 'I have read the Ookla license and privacy notice and accept them for my own personal use';
  OoklaAgree.Checked := False;
  OoklaAgree.Top := Y;
  OoklaAgree.Width := OoklaPage.SurfaceWidth;
  Y := Y + OoklaAgree.Height + ScaleY(10);

  AddLink(OoklaPage, 'Read the Ookla license (EULA)', Y, @OpenEula);
  Y := Y + ScaleY(20);
  AddLink(OoklaPage, 'Read the Ookla terms of use and privacy policy', Y, @OpenTerms);
  Y := Y + ScaleY(28);

  Note := TNewStaticText.Create(OoklaPage);
  Note.Parent := OoklaPage.Surface;
  Note.WordWrap := True;
  Note.Width := OoklaPage.SurfaceWidth;
  Note.Top := Y;
  Note.Caption := 'Prefer to do it yourself? Leave both boxes empty and install the tool from Ookla''s own page. ' +
    'Put speedtest.exe in an "ookla" folder inside the Network Doctor folder, or anywhere on your path, and accept Ookla''s license when it asks.';
  Note.AutoSize := True;
  Y := Y + Note.Height + ScaleY(8);
  AddLink(OoklaPage, 'Open Ookla''s Speedtest CLI page', Y, @OpenCliPage);

  OoklaDownload := CreateDownloadPage('Downloading Ookla Speedtest', 'Fetching the tool from Ookla...', nil);
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  Result := False;
  if PageID = OoklaPage.ID then Result := WizardSilent;
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = OoklaPage.ID then
  begin
    if OoklaOffer.Checked and (not OoklaAgree.Checked) then
    begin
      MsgBox('To download the tool from Ookla you need to tick that you have read and accept its license. Or clear the download box and install it yourself later.', mbInformation, MB_OK);
      Result := False;
    end
    else
      OoklaAgreed := OoklaOffer.Checked and OoklaAgree.Checked;
  end
  else if (CurPageID = wpReady) and OoklaAgreed and (not WizardSilent) then
  begin
    OoklaDownload.Clear;
    OoklaDownload.Add(OoklaZipUrl, 'ookla.zip', OoklaZipSha256);
    OoklaDownload.Show;
    try
      try
        OoklaDownload.Download;
        OoklaFetched := True;
      except
        OoklaFetched := False;
        MsgBox('Ookla''s tool could not be downloaded, so it was not installed. Network Doctor will use its own speed test. You can install the tool yourself later from Ookla''s page.', mbInformation, MB_OK);
      end;
    finally
      OoklaDownload.Hide;
    end;
  end;
end;

procedure UnpackOokla();
var
  Code: Integer;
  Dest: String;
begin
  Dest := ExpandConstant('{app}\ookla');
  if not Exec(ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe'),
    '-NoProfile -ExecutionPolicy Bypass -Command "Expand-Archive -LiteralPath ''' + ExpandConstant('{tmp}\ookla.zip') + ''' -DestinationPath ''' + Dest + ''' -Force"',
    '', SW_HIDE, ewWaitUntilTerminated, Code) or (Code <> 0) then
    Exit;
  // Only now, after the person's own tick, does the tool get told they accepted its license and privacy notice.
  if OoklaAgreed and FileExists(Dest + '\speedtest.exe') then
    OoklaReady := Exec(Dest + '\speedtest.exe', '--accept-license --accept-gdpr -f json --progress=no --servers', '', SW_HIDE, ewWaitUntilTerminated, Code);
end;

procedure WriteChoices();
begin
  SaveStringToFile(ExpandConstant('{app}\choices.json'),
    '{"core":true,"scheduled":' + BoolWord(WizardIsComponentSelected('scheduled')) +
    ',"proactive":' + BoolWord(WizardIsComponentSelected('proactive')) +
    ',"ookla":' + BoolWord(OoklaReady) + '}', False);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  // The alerts helper may be running from an earlier install; stop it so its file can be replaced.
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/IM netdoctorw.exe /F', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Result := '';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if OoklaFetched then UnpackOokla();
    WriteChoices();
  end;
end;

// Uninstall: ask once whether to keep the saved results. Yes (keep) is the default, so a
// silent uninstall keeps them. IT can add /DELETERESULTS=1 to delete them without asking.
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
