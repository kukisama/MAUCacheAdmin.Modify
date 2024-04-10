# Install IIS services.
Write-Host "Installing IIS..."
Install-WindowsFeature -Name Web-Server -IncludeManagementTools

# Create and set the website directory to C:\MAUupdate.
$sitePath = "C:\MAUupdate"
If (-Not (Test-Path $sitePath)) {
    New-Item -Path $sitePath -Type Directory
}

Import-Module WebAdministration

# Use AppCmd.exe to set the physical path of the default website.
$siteName = "Default Web Site"
$sitePathArgument = "physicalPath:" + $sitePath
Set-ItemProperty "IIS:\Sites\$siteName" -Name physicalPath -Value $sitePath

# Enable directory browsing.
Write-Host "Enabling directory browsing..."
Set-WebConfigurationProperty -pspath 'MACHINE/WEBROOT/APPHOST' -filter "system.webServer/directoryBrowse" -name "enabled" -value "True"
Set-WebConfigurationProperty -pspath "IIS:\Sites\$siteName" -filter "system.webServer/directoryBrowse" -name "enabled" -value $true

# Add MIME types for .pkg, .cat, .xml, .mpkg files.
$mimeTypes = @(
    @{extension='.pkg'; mimeType='application/octet-stream'},
    @{extension='.cat'; mimeType='application/octet-stream'},
    @{extension='.xml'; mimeType='application/xml'},
    @{extension='.mpkg'; mimeType='application/octet-stream'}
)

foreach ($type in $mimeTypes) {
    $mimeTypeExists = Get-WebConfigurationProperty -pspath 'MACHINE/WEBROOT/APPHOST' -filter "system.webServer/staticContent/mimeMap" -name "." | Where-Object { $_.fileExtension -eq $type.extension }

    if ($mimeTypeExists -eq $null) {
        Write-Host "Adding MIME type $($type.extension)..."
        Add-WebConfigurationProperty -pspath 'MACHINE/WEBROOT/APPHOST' -location "" -filter "system.webServer/staticContent" -name "." -value @{fileExtension=$type.extension; mimeType=$type.mimeType}
    } else {
        Write-Host "MIME type $($type.extension) already exists."
    }
}


Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Active Setup\Installed Components\{A509B1A7-37EF-4b3f-8CFC-4F3A74704073}" -Name "IsInstalled" -Value 0
Set-ItemProperty -Path "HKLM:\SOFTWARE\Microsoft\Active Setup\Installed Components\{A509B1A8-37EF-4b3f-8CFC-4F3A74704073}" -Name "IsInstalled" -Value 0

Stop-Process -Name Explorer
Start-Process explorer.exe

Write-Host "IE ESC has been disabled and Explorer has been restarted. Please restart your server for the changes to take full effect."

Write-Host "IIS setup and configuration completed."
