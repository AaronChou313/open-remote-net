$ErrorActionPreference = "Stop"
$Repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$Destination = Join-Path $env:LOCALAPPDATA "OpenRemoteHub\ORN"
$Config = Join-Path $env:APPDATA "orn-agent\config.json"
New-Item -ItemType Directory -Force -Path $Destination | Out-Null
$Binary = Join-Path $Destination "orn-agent.exe"
Copy-Item (Join-Path $Repo "dist\orn-agent-windows-amd64.exe") $Binary -Force
$Action = New-ScheduledTaskAction -Execute $Binary -Argument "-config `"$Config`""
$Trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$Principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited
$Settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName "OpenRemoteHub ORN" -Action $Action -Trigger $Trigger -Principal $Principal -Settings $Settings -Force | Out-Null
Start-ScheduledTask -TaskName "OpenRemoteHub ORN"
Start-Sleep -Seconds 2
& $Binary -config $Config open
