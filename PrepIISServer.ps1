# Install IIS services.
Write-Host "Installing IIS..."
Install-WindowsFeature -Name Web-Server -IncludeManagementTools

# Create and set the website directory to C:\MAUupdate.
$sitePath = "C:\MAUupdate"
If (-Not (Test-Path $sitePath)) {
    New-Item -Path $sitePath -Type Directory
}

# Use AppCmd.exe to set the physical path of the default website.
$siteName = "Default Web Site"
$sitePathArgument = "physicalPath:" + $sitePath
& "$env:windir\system32\inetsrv\appcmd.exe" set site /site.name:"$siteName" /[$sitePathArgument]

# Enable directory browsing.
Write-Host "Enabling directory browsing..."
& "$env:windir\system32\inetsrv\appcmd.exe" set config /section:directoryBrowse /enabled:true

# Add MIME types for .pkg, .cat, .xml, .mpkg files.
Write-Host "Adding MIME types..."
$mimeTypes = @(
    @{extension='.pkg'; mimeType='application/octet-stream'},
    @{extension='.cat'; mimeType='application/octet-stream'},
    @{extension='.xml'; mimeType='application/xml'},
    @{extension='.mpkg'; mimeType='application/octet-stream'}
)

foreach ($type in $mimeTypes) {
    & "$env:windir\system32\inetsrv\appcmd.exe" set config /section:staticContent /+"[fileExtension='$($type.extension)',mimeType='$($type.mimeType)']"
}

# Disable IE ESC
Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Active Setup\Installed Components\{A509B1A7-37EF-4b3f-8CFC-4F3A74704073}" -Name "IsInstalled" -Value 0
Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Active Setup\Installed Components\{A509B1A8-37EF-4b3f-8CFC-4F3A74704073}" -Name "IsInstalled" -Value 0

Stop-Process -Name Explorer
Start-Process explorer.exe

Write-Host "IE ESC has been disabled and Explorer has been restarted. Please restart your server for the changes to take full effect."

Write-Host "IIS setup and configuration completed."
