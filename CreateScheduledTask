$workDir="c:"
# one time
$taskScriptPath = "$workDir\MAU.ps1"
$triggerTime =(Get-Date).AddHours(1) 
$action = New-ScheduledTaskAction -Execute "Powershell.exe" -Argument "-ExecutionPolicy Bypass -File  `"$taskScriptPath`"" 
$trigger = New-ScheduledTaskTrigger -At $triggerTime -Once 
$principal = New-ScheduledTaskPrincipal -UserId "NT AUTHORITY\SYSTEM" -LogonType ServiceAccount -RunLevel Highest 
Register-ScheduledTask -Action $action -Trigger $trigger -Principal $principal -TaskName "MAU update" -Description "for MAU"

# weekly
$taskScriptPath = "$workDir\MAU.ps1"
$triggerTime = "01:00"
$action = New-ScheduledTaskAction -Execute "Powershell.exe" -Argument "-ExecutionPolicy Bypass -File `"$taskScriptPath`""
$trigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Saturday -At $triggerTime
$principal = New-ScheduledTaskPrincipal -UserId "NT AUTHORITY\SYSTEM" -LogonType ServiceAccount -RunLevel Highest
Register-ScheduledTask -Action $action -Trigger $trigger -Principal $principal -TaskName "MAU update" -Description "for MAU"
