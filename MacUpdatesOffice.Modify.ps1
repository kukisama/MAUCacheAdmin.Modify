param (
  # 用户可以在运行脚本时指定工作路径，如果未指定则使用默认值"c:\MACupdate"
  [string]$workPath = "C:\MAUCacheAdmin.Modify-main",
  # 用户可以在运行脚本时指定MAU存储离线文件的路径，如果未指定则使用默认值"C:\inetpub\wwwroot\maunew6"
  [string]$maupath = "C:\inetpub\wwwroot\maucache",
  # 用户可以在运行脚本时指定MAU临时路径，如果未指定则使用默认值"c:\MACupdate\temp"
  [string]$mautemppath = "c:\MAUCacheAdmin.Modify-main\temp"

)

# Change to the specified work directory
Set-Location $workPath


$filesToDelete = @(
    "Lync Installer.pkg",
    "MicrosoftTeams.pkg",
    "Teams_osx.pkg",
    "wdav-upgrade.pkg",
    "*.xml",
    "builds.txt",
    "*.cat"
)

foreach ($file in $filesToDelete) {
    $fullPathPattern = Join-Path -Path $maupath -ChildPath $file

    $matchingFiles = Get-ChildItem -Path $fullPathPattern -ErrorAction SilentlyContinue  -Recurse

    foreach ($matchingFile in $matchingFiles) {
        if (Test-Path $matchingFile.FullName) {
            Write-Host "Deleting file: $($matchingFile.FullName)"
            Remove-Item $matchingFile.FullName -Force
        }
    }
}

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
Save-oldMAUCollaterals -MAUApps $apps -CachePath $maupath  

# Download the MAU cache for each app
$apps | ForEach-Object {
  $dlJobs = Get-MAUCacheDownloadJobs -MAUApps $_ -DeltaFromBuildLimiter $builds 
  Invoke-MAUCacheDownload -MAUCacheDownloadJobs $dlJobs -CachePath $maupath -ScratchPath $mautemppath -Force
}
