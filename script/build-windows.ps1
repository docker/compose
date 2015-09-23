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
.\venv\Scripts\pip install pypiwin32==219
.\venv\Scripts\pip install -r requirements.txt
.\venv\Scripts\pip install --no-deps .

# TODO: pip warns when installing from a git sha, so we need to set ErrorAction to
# 'Continue'.  See
# https://github.com/pypa/pip/blob/fbc4b7ae5fee00f95bce9ba4b887b22681327bb1/pip/vcs/git.py#L77
# This can be removed once pyinstaller 3.x is released and we upgrade 
$ErrorActionPreference = "Continue"
.\venv\Scripts\pip install --allow-external pyinstaller -r requirements-build.txt

# Build binary
# pyinstaller has lots of warnings, so we need to run with ErrorAction = Continue
.\venv\Scripts\pyinstaller .\docker-compose.spec
$ErrorActionPreference = "Stop"

Move-Item -Force .\dist\docker-compose .\dist\docker-compose-Windows-x86_64.exe
.\dist\docker-compose-Windows-x86_64.exe --version
