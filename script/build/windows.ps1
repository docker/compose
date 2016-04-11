# Builds the Windows binary.
#
# From a fresh 64-bit Windows 10 install, prepare the system as follows:
#
# 1. Install Git:
#
#        http://git-scm.com/download/win
#
# 2. Install Python 2.7.10:
#
#        https://www.python.org/downloads/
#
# 3. Append ";C:\Python27;C:\Python27\Scripts" to the "Path" environment variable:
#
#        https://www.microsoft.com/resources/documentation/windows/xp/all/proddocs/en-us/sysdm_advancd_environmnt_addchange_variable.mspx?mfr=true
#
# 4. In Powershell, run the following commands:
#
#        $ pip install virtualenv
#        $ Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
#
# 5. Clone the repository:
#
#        $ git clone https://github.com/docker/compose.git
#        $ cd compose
#
# 6. Build the binary:
#
#        .\script\build\windows.ps1

$ErrorActionPreference = "Stop"

# Remove virtualenv
if (Test-Path venv) {
    Remove-Item -Recurse -Force .\venv
}

# Remove .pyc files
Get-ChildItem -Recurse -Include *.pyc | foreach ($_) { Remove-Item $_.FullName }

# Create virtualenv
virtualenv .\venv

# pip and pyinstaller generate lots of warnings, so we need to ignore them
$ErrorActionPreference = "Continue"

# Install dependencies
.\venv\Scripts\pip install pypiwin32==219
.\venv\Scripts\pip install -r requirements.txt
.\venv\Scripts\pip install --no-deps .
.\venv\Scripts\pip install --allow-external pyinstaller -r requirements-build.txt

git rev-parse --short HEAD | out-file -encoding ASCII compose\GITSHA

# Build binary
.\venv\Scripts\pyinstaller .\docker-compose.spec
$ErrorActionPreference = "Stop"

Move-Item -Force .\dist\docker-compose.exe .\dist\docker-compose-Windows-x86_64.exe
.\dist\docker-compose-Windows-x86_64.exe --version
