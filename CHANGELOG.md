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

[Release diff](https://github.com/docker/compose-cli/compare/<LAST TAG>...<THIS TAG>)
-->

## 0.1.4 - 2020-06-26

First public beta release of the Docker CLI with
[ACI](https://azure.microsoft.com/en-us/services/container-instances/)
integration!

This release includes:
* Initial support for deploying containers and Compose applications to Azure Container Instances (ACI)
* A gRPC API for managing contexts and Azure containers

### Known issues
* Mapping a container port to a different host port is not supported in ACI (i.e.: `docker run -p 80:8080`). You can only expose the container port to the same port on the host.
* Exec currently only allows interactive sessions with a terminal (`exec -t`), not specify commands in the command line.
* `docker run` detaches from the container by default, even if `-d` is not specified. Logs can be seen later on with command `docker log <CONTAINER_ID>`.
* Replicas are not supported when deploying Compose application. One container will be run for each Compose service. Several services cannot expose the same port.
* Windows Containers are not supported on ACI in multi-container compose applications.