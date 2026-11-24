; Network Doctor installer for Windows. Built with Inno Setup 6.
; Purpose: install the program, and only the optional parts the person picks. Run it again to repair: it puts back
;   what the person chose and removes what they did not. The uninstaller (Apps and features) removes it all,
;   the saved results and settings in ProgramData too, with no question, so a new install starts fresh.
;   First it puts back every fix still applied, with the installed program's own undo; when one cannot be put back,
;   only its undo notes stay in ProgramData and the person is told. IT can keep the results with /KEEPRESULTS (bare
;   or any value but empty or 0); the old /DELETERESULTS=1 is still accepted and changes nothing.
; Owner: Rafiq. Version 0.7.2 (0.7.2: a setting changed by hand since its fix is left as set and told, not a fault; notes holding only {} count as none, 2026-11-23; 0.7.1: the undo names the results folder, says when the results are kept, and an old note is deleted first, 2026-11-23; 0.7.0: removal puts back every fix still applied first; /KEEPRESULTS keeps for any value but empty or 0, 2026-11-23; 0.6.0: removal takes away everything, results and settings too, with no question; only /KEEPRESULTS=1 keeps the results, 2026-11-23; 0.5.0: the icons open the check as a page in the web browser, with no console window, 2026-11-19; 0.4.0: a finish screen that says what to do next, 2026-11-19; 0.3.0: setup lines made honest, 2026-11-19). Requirements ND-110 to ND-120, ND-209 to ND-211, ND-256, ND-257, ND-260, ND-261, ND-263 and ND-316.
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
AlwaysShowDirOnReadyPage=yes
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
Name: "core"; Description: "Check on demand (always included). Run a full check any time you want one. Your data never leaves your computer. The speed test and a few small requests do go out: see What goes over the internet in the guide."; Types: reactive full custom; Flags: fixed
Name: "scheduled"; Description: "Weekly check. Once a week, Monday at 0900, saves your speed to a file. Your data never leaves your computer. The speed test and a few small requests do go out: see What goes over the internet in the guide."; Types: full
Name: "proactive"; Description: "Alerts. A small helper starts when you log on and runs all the time. It alerts you when your connection drops, stays slow (if you set your plan speed), or a new device appears. Alerts are kept in a file and shown on the screen, and setup can turn off the pop-ups and the file. Your data never leaves your computer. The speed test and a few small requests do go out: see What goes over the internet in the guide."; Types: full

[Dirs]
Name: "{commonappdata}\NetworkDoctor"; Permissions: users-modify

[Files]
Source: "{#StageDir}\netdoctor.exe"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "{#StageDir}\netdoctorw.exe"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "{#StageDir}\GUIDE.md"; DestDir: "{app}"; Components: core; Flags: ignoreversion
Source: "installed.marker"; DestDir: "{app}"; Components: core; Flags: ignoreversion

[Icons]
; One press opens a page in the web browser that runs a full check, with no console window, so nobody has to
; open a terminal or type anything. The program quits by itself when the page is closed (ND-261, ND-263).
Name: "{autoprograms}\Network Doctor"; Filename: "{app}\netdoctorw.exe"; Parameters: "open"; WorkingDir: "{app}"; IconFilename: "{app}\netdoctor.exe"; Comment: "Check your internet connection"; Components: core
Name: "{autodesktop}\Network Doctor"; Filename: "{app}\netdoctorw.exe"; Parameters: "open"; WorkingDir: "{app}"; IconFilename: "{app}\netdoctor.exe"; Comment: "Check your internet connection"; Components: core

[Run]
Filename: "schtasks.exe"; Parameters: "/Create /TN ""Network Doctor weekly check"" /TR ""\""{app}\netdoctorw.exe\"" weekly"" /SC WEEKLY /D MON /ST 09:00 /RU ""BUILTIN\Users"" /F"; Flags: runhidden; Components: scheduled; StatusMsg: "Setting up the weekly check..."
; schtasks has no switch for these settings (ND-289): no run-time limit and running on battery for both
; tasks; the weekly task runs as soon as it can after a missed start; the alerts task is started again a
; minute after a failure (3 times) and its log-on start repeats every 5 minutes, so it comes back
; whatever stopped it.
Filename: "powershell.exe"; Parameters: "-NoProfile -NonInteractive -Command ""$t = Get-ScheduledTask -TaskName 'Network Doctor weekly check'; $t.Settings.ExecutionTimeLimit = 'PT0S'; $t.Settings.DisallowStartIfOnBatteries = $false; $t.Settings.StopIfGoingOnBatteries = $false; $t.Settings.StartWhenAvailable = $true; Set-ScheduledTask -InputObject $t | Out-Null"""; Flags: runhidden; Components: scheduled; StatusMsg: "Setting up the weekly check..."
Filename: "schtasks.exe"; Parameters: "/Create /TN ""Network Doctor alerts"" /TR ""\""{app}\netdoctorw.exe\"" watch"" /SC ONLOGON /RU ""BUILTIN\Users"" /F"; Flags: runhidden; Components: proactive; StatusMsg: "Setting up alerts..."
Filename: "powershell.exe"; Parameters: "-NoProfile -NonInteractive -Command ""$t = Get-ScheduledTask -TaskName 'Network Doctor alerts'; $t.Settings.ExecutionTimeLimit = 'PT0S'; $t.Settings.DisallowStartIfOnBatteries = $false; $t.Settings.StopIfGoingOnBatteries = $false; $t.Settings.RestartCount = 3; $t.Settings.RestartInterval = 'PT1M'; $t.Settings.MultipleInstances = 'IgnoreNew'; $t.Triggers[0].Repetition.Interval = 'PT5M'; Set-ScheduledTask -InputObject $t | Out-Null"""; Flags: runhidden; Components: proactive; StatusMsg: "Setting up alerts..."
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
  KeepResults: Boolean;   // IT asked with /KEEPRESULTS to keep the saved results
  FixesLeft: Boolean;     // a fix could not be put back, so its undo notes stay
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
  OoklaOffer.Height := ScaleY(22);
  OoklaOffer.Width := OoklaPage.SurfaceWidth;
  Y := Y + OoklaOffer.Height + ScaleY(10);

  OoklaAgree := TNewCheckBox.Create(OoklaPage);
  OoklaAgree.Parent := OoklaPage.Surface;
  OoklaAgree.Caption := 'I have read the Ookla license and privacy notice and accept them for my own personal use';
  OoklaAgree.Checked := False;
  OoklaAgree.Top := Y;
  OoklaAgree.Height := ScaleY(22);
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

// The Ready page lists short names, not the long part descriptions, which run past the edge of the page.
function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo, MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
var
  S: String;
begin
  S := MemoTypeInfo + NewLine + NewLine;
  if MemoDirInfo <> '' then S := S + MemoDirInfo + NewLine + NewLine;
  S := S + 'Parts to install:' + NewLine + Space + 'Check on demand';
  if WizardIsComponentSelected('scheduled') then S := S + NewLine + Space + 'Weekly check';
  if WizardIsComponentSelected('proactive') then S := S + NewLine + Space + 'Alerts';
  if OoklaAgreed then
    S := S + NewLine + NewLine + 'Speed tool:' + NewLine + Space + 'Download Ookla Speedtest from Ookla'
  else
    S := S + NewLine + NewLine + 'Speed tool:' + NewLine + Space + 'Not added (Network Doctor uses its own speed test)';
  Result := S;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if OoklaFetched then UnpackOokla();
    WriteChoices();
  end;
end;

// The last screen says what to do next, built from the parts chosen this run (a repair run too), so it
// never claims a part the person did not pick. A silent install never shows this page.
procedure CurPageChanged(CurPageID: Integer);
var
  S, NL: String;
begin
  if CurPageID <> wpFinished then Exit;
  NL := #13#10;
  S := 'Network Doctor is installed.' + NL + NL;
  S := S + 'To run a check, press the Network Doctor icon on your desktop or in the Start menu. A page opens in your web browser, runs the check and shows the answer. Close the browser tab when you are done.' + NL + NL;
  if WizardIsComponentSelected('scheduled') then S := S + 'Weekly check: on. It saves your speed to a file every Monday at 0900.' + NL
  else S := S + 'Weekly check: not chosen.' + NL;
  if WizardIsComponentSelected('proactive') then S := S + 'Alerts: on. They start the next time you log on.' + NL + NL
  else S := S + 'Alerts: not chosen.' + NL + NL;
  S := S + 'The program is in ' + ExpandConstant('{app}') + '.' + NL;
  S := S + 'Your results are saved in ' + ExpandConstant('{commonappdata}') + '\NetworkDoctor' + '.' + NL + NL;
  S := S + 'To remove it: Windows Settings, Apps, Installed apps, Network Doctor, Uninstall. It takes away everything, your saved results and settings too, so a new install starts fresh. It first tries to put back any setting a fix changed.' + NL + NL;
  S := S + 'The full guide is GUIDE.md in ' + ExpandConstant('{app}') + '.';
  WizardForm.FinishedLabel.Caption := S;
  WizardForm.AdjustLabelHeight(WizardForm.FinishedLabel);
end;

// KeepResultsAsked says whether IT gave /KEEPRESULTS (or -KEEPRESULTS), bare or with any value but
// empty or 0, in any case. {param:} matches only /NAME=value, so the switches are read here.
function KeepResultsAsked(): Boolean;
var
  I: Integer;
  P, V: String;
begin
  Result := False;
  for I := 1 to ParamCount do
  begin
    P := Uppercase(Trim(ParamStr(I)));
    if Copy(P, 1, 1) = '-' then P := '/' + Copy(P, 2, Length(P));
    if P = '/KEEPRESULTS' then
      Result := True
    else if Copy(P, 1, 13) = '/KEEPRESULTS=' then
    begin
      V := Trim(Copy(P, 14, Length(P)));
      Result := (V <> '') and (V <> '0');
    end;
  end;
end;

// Uninstall asks nothing of its own (Inno Setup's own "remove it?" question stays): it removes
// everything, the whole results folder too (saved results, the alert log, the known-devices list,
// the device choices and the settings), silent or not, so a new install starts fresh. Only IT's
// /KEEPRESULTS keeps that folder. The old /DELETERESULTS=1 is still accepted, as Inno accepts any
// /NAME=value, and changes nothing, since deleting is the default; with both, the folder is kept.
function InitializeUninstall(): Boolean;
begin
  KeepResults := KeepResultsAsked();
  FixesLeft := False;
  Result := True;
end;

// PutBackFixes runs the installed program's own undo of every fix still applied, once, after the
// person confirmed and before the tasks, the program or the results go. A fault it reports, or a
// program that will not start while undo notes exist, means only the undo notes stay; the plain
// lines go to the log and, unless the removal is silent, show once with only OK. A setting the person
// changed by hand since its fix is left as set and only told the same way; removal goes on as normal. Inno Setup cannot
// set the uninstaller's own exit code, so the kept note file and the log are the record.
procedure PutBackFixes();
var
  Code: Integer;
  Dir, NotePath, Args, Note: String;
  NoteA, SavedA: AnsiString;
begin
  if not FileExists(ExpandConstant('{commonappdata}\NetworkDoctor\fix-saved.json')) then
    Exit;
  Dir := ExpandConstant('{commonappdata}\NetworkDoctor');
  // Notes with nothing recorded are no notes: nothing to put back, even if the program cannot start.
  if LoadStringFromFile(Dir + '\fix-saved.json', SavedA) and (Trim(String(SavedA)) = '{}') then
    Exit;
  NotePath := Dir + '\fix-not-undone.txt';
  // A note left by an earlier removal is never shown again: it goes before the undo runs.
  DeleteFile(NotePath);
  // The results folder is named, so a NETDOCTOR_DIR set for the whole computer cannot point the
  // undo at other notes while the real ones here are deleted; --kept keeps the note truthful.
  Args := 'fix undo-all --dir "' + Dir + '" --note "' + NotePath + '"';
  if KeepResults then Args := Args + ' --kept';
  if not Exec(ExpandConstant('{app}\netdoctor.exe'), Args, '',
    SW_HIDE, ewWaitUntilTerminated, Code) or (Code <> 0) then
  begin
    FixesLeft := True;
    if LoadStringFromFile(NotePath, NoteA) and (Trim(String(NoteA)) <> '') then
      Note := String(NoteA)
    else
      Note := 'Network Doctor could not put back every change its fixes made. ' +
        'The notes for them are kept in ' + ExpandConstant('{commonappdata}\NetworkDoctor') + ', so they can still be put back. ' +
        'To do that, install Network Doctor again and use its undo, as the guide shows under Fix a problem. Or change the setting back by hand.';
    Log(Note);
    if not UninstallSilent then
      SuppressibleMsgBox(Note, mbInformation, MB_OK, IDOK);
  end
  else if LoadStringFromFile(NotePath, NoteA) and (Trim(String(NoteA)) <> '') then
  begin
    Note := String(NoteA);
    Log(Note);
    if not UninstallSilent then
      SuppressibleMsgBox(Note, mbInformation, MB_OK, IDOK);
  end;
end;

// KeepOnlyUndoNotes deletes everything in the results folder but the undo notes, so a fix not put
// back can still be put back later.
procedure KeepOnlyUndoNotes(Dir: String);
var
  FR: TFindRec;
  Name: String;
begin
  if not FindFirst(Dir + '\*', FR) then
    Exit;
  try
    repeat
      Name := FR.Name;
      if (Name <> '.') and (Name <> '..') and (CompareText(Name, 'fix-saved.json') <> 0) and
        (CompareText(Name, 'fix-log.txt') <> 0) and (CompareText(Name, 'fix-enabled-here.txt') <> 0) and
        (CompareText(Name, 'fix-not-undone.txt') <> 0) then
      begin
        if FR.Attributes and FILE_ATTRIBUTE_DIRECTORY <> 0 then
          DelTree(Dir + '\' + Name, True, True, True)
        else
          DeleteFile(Dir + '\' + Name);
      end;
    until not FindNext(FR);
  finally
    FindClose(FR);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if (CurUninstallStep = usUninstall) then
    PutBackFixes();
  if (CurUninstallStep = usPostUninstall) and (not KeepResults) then
  begin
    if FixesLeft then
      KeepOnlyUndoNotes(ExpandConstant('{commonappdata}\NetworkDoctor'))
    else
      DelTree(ExpandConstant('{commonappdata}\NetworkDoctor'), True, True, True);
  end;
end;
