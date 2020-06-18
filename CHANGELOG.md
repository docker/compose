# Changelog

<!-- TEMPLATE
## x.y.z - YYYY-MM-DD

Release headlines

### Added
*

### Changed
*

### Removed
*

### Fixed
*

### Known issues
*

[Release diff](https://github.com/docker/api/compare/<LAST TAG>...<THIS TAG>)
-->

## 0.1.z - 2020-06-DD

First public release beta of the Docker CLI with
[ACI](https://azure.microsoft.com/en-us/services/container-instances/)
integration!

This release includes:
* Initial support for deploying containers to Azure Container Instances (ACI)
* A gRPC API for managing contexts and Azure containers

### Known issues
* Mapping a container port to a different host port is not current supported (i.e.: `docker run -p 80:8080`)
* Exec currently only allows interactive sessions with a terminal (`exec -t`), not specify commands in the command line
* `docker run` detaches from the container by default, even if `-d` is not specified
