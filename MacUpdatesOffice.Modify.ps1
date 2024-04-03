param (
  # 用户可以在运行脚本时指定工作路径，如果未指定则使用默认值"c:\MACupdate"
  [string]$workPath = "c:\MACupdate",
  # 用户可以在运行脚本时指定MAU存储离线文件的路径，如果未指定则使用默认值"C:\inetpub\wwwroot\maunew6"
  [string]$maupath = "C:\inetpub\wwwroot\maunew6",
  # 用户可以在运行脚本时指定MAU临时路径，如果未指定则使用默认值"c:\MACupdate\temp"
  [string]$mautemppath = "c:\MACupdate\temp"
)

# Change to the specified work directory
Set-Location $workPath

# Import the MAUCacheAdmin module
Import-Module .\PSModule\MAUCacheAdmin\MAUCacheAdmin.psm1

# Add the System.Net.Http assembly for HTTP operations
Add-Type -AssemblyName System.Net.Http

# Get the MAU production builds
$builds = Get-MAUProductionBuilds

# Get the MAU apps for the Production channel
$apps = Get-MAUApps -Channel Production 

# Save the MAU collaterals
Save-MAUCollaterals -MAUApps $apps -CachePath $maupath -isProd $true

# Download the MAU cache for each app
$apps | ForEach-Object {
  $dlJobs = Get-MAUCacheDownloadJobs -MAUApps $_ -DeltaFromBuildLimiter $builds 
  Invoke-MAUCacheDownload -MAUCacheDownloadJobs $dlJobs -CachePath $maupath -ScratchPath $mautemppath -Force
}
