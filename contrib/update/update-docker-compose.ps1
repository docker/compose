# Self-elevate the script if required
# http://www.expta.com/2017/03/how-to-self-elevate-powershell-script.html
If (-Not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] 'Administrator')) {
    If ([int](Get-CimInstance -Class Win32_OperatingSystem | Select-Object -ExpandProperty BuildNumber) -ge 6000) {
        $CommandLine = "-File `"" + $MyInvocation.MyCommand.Path + "`" " + $MyInvocation.UnboundArguments
        Start-Process -FilePath PowerShell.exe -Verb Runas -ArgumentList $CommandLine
        Exit
    }
}

$SectionSeparator = "--------------------------------------------------"

# Update docker-compose if required
Function UpdateDockerCompose() {
    Write-Host "Updating docker-compose if required..."
    Write-Host $SectionSeparator

    # Find the installed docker-compose.exe location
    Try {
        $DockerComposePath = Get-Command docker-compose.exe -ErrorAction Stop | `
            Select-Object -First 1 -ExpandProperty Definition
    }
    Catch {
        Write-Host "Error: Could not find path to docker-compose.exe" `
            -ForegroundColor Red
        Return $false
    }

    # Prefer/enable TLS 1.2
    # https://stackoverflow.com/a/48030563/153079
    [Net.ServicePointManager]::SecurityProtocol = "tls12, tls11, tls"

    # Query for the latest release version
    Try {
        $URI = "https://api.github.com/repos/docker/compose/releases/latest"
        $LatestComposeVersion = [System.Version](Invoke-RestMethod -Method Get -Uri $URI).tag_name
    }
    Catch {
        Write-Host "Error: Query for the latest docker-compose release version failed" `
            -ForegroundColor Red
        Return $false
    }

    # Check the installed version and compare with latest release
    $UpdateDockerCompose = $false
    Try {
        $InstalledComposeVersion = `
            [System.Version]((docker-compose.exe version --short) | Out-String)

        If ($InstalledComposeVersion -eq $LatestComposeVersion) {
            Write-Host ("Installed docker-compose version ({0}) same as latest ({1})." `
                -f $InstalledComposeVersion.ToString(), $LatestComposeVersion.ToString())
        }
        ElseIf ($InstalledComposeVersion -lt $LatestComposeVersion) {
            Write-Host ("Installed docker-compose version ({0}) older than latest ({1})." `
                -f $InstalledComposeVersion.ToString(), $LatestComposeVersion.ToString())
            $UpdateDockerCompose = $true
        }
        Else {
            Write-Host ("Installed docker-compose version ({0}) newer than latest ({1})." `
                -f $InstalledComposeVersion.ToString(), $LatestComposeVersion.ToString()) `
                -ForegroundColor Yellow
        }
    }
    Catch {
        Write-Host `
            "Warning: Couldn't get docker-compose version, assuming an update is required..." `
            -ForegroundColor Yellow
        $UpdateDockerCompose = $true
    }

    If (-Not $UpdateDockerCompose) {
        # Nothing to do!
        Return $false
    }

    # Download the latest version of docker-compose.exe
    Try {
        $RemoteFileName = "docker-compose-Windows-x86_64.exe"
        $URI = ("https://github.com/docker/compose/releases/download/{0}/{1}" `
            -f $LatestComposeVersion.ToString(), $RemoteFileName)
        Invoke-WebRequest -UseBasicParsing -Uri $URI `
            -OutFile $DockerComposePath
        Return $true
    }
    Catch {
        Write-Host ("Error: Failed to download the latest version of docker-compose`n{0}" `
            -f $_.Exception.Message) -ForegroundColor Red
        Return $false
    }

    Return $false
}

If (UpdateDockerCompose) {
    Write-Host "Updated to latest-version of docker-compose, running update again to verify.`n"
    If (UpdateDockerCompose) {
        Write-Host "Error: Should not have updated twice." -ForegroundColor Red
    }
}

# Assuming elevation popped up a new powershell window, pause so the user can see what happened
# https://stackoverflow.com/a/22362868/153079
Function Pause ($Message = "Press any key to continue . . . ") {
    If ((Test-Path variable:psISE) -and $psISE) {
        $Shell = New-Object -ComObject "WScript.Shell"
        $Shell.Popup("Click OK to continue.", 0, "Script Paused", 0)
    }
    Else {
        Write-Host "`n$SectionSeparator"
        Write-Host -NoNewline $Message
        [void][System.Console]::ReadKey($true)
        Write-Host
    }
}
Pause
