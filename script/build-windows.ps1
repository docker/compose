$ErrorActionPreference = "Stop"
Set-PSDebug -trace 1

# Remove virtualenv
if (Test-Path venv) {
    Remove-Item -Recurse -Force .\venv
}

# Remove .pyc files
Get-ChildItem -Recurse -Include *.pyc | foreach ($_) { Remove-Item $_.FullName }

# Create virtualenv
virtualenv .\venv

# Install dependencies
.\venv\Scripts\easy_install "http://sourceforge.net/projects/pywin32/files/pywin32/Build%20219/pywin32-219.win32-py2.7.exe/download"
.\venv\Scripts\pip install -r requirements.txt
.\venv\Scripts\pip install -r requirements-build.txt
.\venv\Scripts\pip install .

# Build binary
.\venv\Scripts\pyinstaller .\docker-compose.spec
Move-Item -Force .\dist\docker-compose .\dist\docker-compose-Windows-x86_64.exe
.\dist\docker-compose-Windows-x86_64.exe --version
