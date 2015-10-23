# Run docker-compose in a container via boot2docker.
#
# The current directory will be mirrored as a volume and additional
# volumes (or any other options) can be mounted by using
# $Env:DOCKER_COMPOSE_OPTIONS.

if ($Env:DOCKER_COMPOSE_VERSION -eq $null -or $Env:DOCKER_COMPOSE_VERSION.Length -eq 0) {
    $Env:DOCKER_COMPOSE_VERSION = "latest"
}

if ($Env:DOCKER_COMPOSE_OPTIONS -eq $null) {
    $Env:DOCKER_COMPOSE_OPTIONS = ""
}

if (-not $Env:DOCKER_HOST) {
    docker-machine env --shell=powershell default | Invoke-Expression
    if (-not $?) { exit $LastExitCode }
}

$local="/$($PWD -replace '^(.):(.*)$', '"$1".ToLower()+"$2".Replace("\","/")' | Invoke-Expression)"
docker run --rm -ti -v /var/run/docker.sock:/var/run/docker.sock -v "${local}:$local" -w "$local" $Env:DOCKER_COMPOSE_OPTIONS "docker/compose:$Env:DOCKER_COMPOSE_VERSION" $args
exit $LastExitCode
