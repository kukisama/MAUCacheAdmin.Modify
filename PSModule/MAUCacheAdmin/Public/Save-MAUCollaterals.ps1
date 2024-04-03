function Save-MAUCollaterals {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute("PSUseSingularNouns", "")]
    [CmdletBinding()]
    param (
        [Parameter(Mandatory = $true)]
        [PSCustomObject[]]
        $MAUApps,
        
        [Parameter(Mandatory=$true)]
        [string]
        $CachePath,
        
        [Parameter(Mandatory=$false)]
        [bool]
        $isProd = $false
    )

    $logPrefix = "$($MyInvocation.MyCommand):"

    # Ensure the cache path exists, create if not
    if (-not (Test-Path -Path $CachePath)) {
        try {
            New-Item -ItemType Directory -Path $CachePath -Force | Out-Null
            Write-Host "The target Cache Path has been successfully created: $CachePath"
        } catch {
            throw "Failed to create the target Cache Path: $_"
        }
    } 

    # Determine the base path for saving files based on the isProd flag
    $basePath = $CachePath
    if (-not $isProd) {
        $basePath = Join-Path -Path $CachePath -ChildPath "collateral"
        $null = New-Item -Path $basePath -ItemType Directory -Force
    }

    foreach ($mauApp in $MAUApps) {
        $ver = $mauApp.VersionInfo.Version
        $verDir = $basePath
        if (-not $isProd) {
            $verDir = Join-Path -Path $basePath -ChildPath $ver
            $null = New-Item -Path $verDir -ItemType Directory -Force
        }

        Write-Verbose "$logPrefix Saving $($mauApp.AppID) collaterals to $verDir"

        $originalUri = $mauApp.CollateralURIs.CAT.OriginalString
        $lastSlashIndex = $originalUri.LastIndexOf('/')
        $baseUri = $originalUri.Substring(0, $lastSlashIndex + 1)
        $fileName = $originalUri.Substring($lastSlashIndex + 1)
        $fileNameParts = $fileName -split '\.'
        $newFileName = $fileNameParts[0] + "_" + $ver + "." + $fileNameParts[1]
        $newUri = New-Object System.Uri($baseUri + $newFileName)
        $newxmlUri = New-Object System.Uri($baseUri + $fileNameParts[0] + "_" + $ver + ".xml")
        #ls C:\inetpub\wwwroot\maunew3 -File -Recurse
        $collateralURIs = @($newxmlUri, $newUri, $mauApp.CollateralURIs.AppXML, $mauApp.CollateralURIs.CAT, $mauApp.CollateralURIs.ChkXml) | Where-Object { $null -ne $_ }
        $collateralURIs | Foreach-Object {Invoke-HttpClientDownload -Uri $_ -Path $verDir -UseRemoteLastModified -Force}
        
    }
}
