; Xiaomi Cam for Windows – Inno Setup script (make windows: ISCC under Wine).
; Installs for the current user (no administrator prompt) to %LOCALAPPDATA%\Programs,
; with ffmpeg next to the app (the app runs ffmpeg/ffprobe from its own folder).

#ifndef Version
  #define Version "0.0.0"
#endif
#ifndef Build
  #define Build "..\..\dist\windows"
#endif

[Setup]
AppId={{6B0F3C1E-9A7D-4C55-8E21-5D3A9F4B7C10}
AppName=Xiaomi Cam
AppVersion={#Version}
AppVerName=Xiaomi Cam {#Version}
AppPublisher=genosajd
AppPublisherURL=https://github.com/genosajd/xiaomi-pc-view
AppSupportURL=https://github.com/genosajd/xiaomi-pc-view/issues
AppUpdatesURL=https://github.com/genosajd/xiaomi-pc-view/releases
DefaultDirName={autopf}\Xiaomi Cam
DefaultGroupName=Xiaomi Cam
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputBaseFilename=xiaomi-cam-{#Version}-windows-setup
SetupIconFile=xiaomi-cam.ico
UninstallDisplayIcon={app}\xiaomi-cam.exe
UninstallDisplayName=Xiaomi Cam
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
CloseApplications=force
RestartApplications=no
LicenseFile=..\..\LICENSE

[Languages]
Name: "pl"; MessagesFile: "compiler:Languages\Polish.isl"
Name: "en"; MessagesFile: "compiler:Default.isl"

[CustomMessages]
pl.Autostart=Uruchamiaj przy logowaniu (w zasobniku)
en.Autostart=Start at login (in the tray)
pl.Launch=Uruchom Xiaomi Cam
en.Launch=Start Xiaomi Cam

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"
Name: "autostart"; Description: "{cm:Autostart}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#Build}\xiaomi-cam.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Build}\ffmpeg\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs

[Icons]
Name: "{autoprograms}\Xiaomi Cam"; Filename: "{app}\xiaomi-cam.exe"
Name: "{autodesktop}\Xiaomi Cam"; Filename: "{app}\xiaomi-cam.exe"; Tasks: desktopicon

[Registry]
; the same value as Settings → Start at login of the app (autostart_windows.go)
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "Xiaomi Cam"; ValueData: """{app}\xiaomi-cam.exe"" --tray"; Tasks: autostart; Flags: uninsdeletevalue
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueName: "Xiaomi Cam"; Flags: dontcreatekey uninsdeletevalue

[Run]
Filename: "{app}\xiaomi-cam.exe"; Description: "{cm:Launch}"; Flags: nowait postinstall skipifsilent
