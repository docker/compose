---
title: ACI integration Compose features
description: Reference list of compose ACI features
keywords: Docker, Azure, Integration, ACI, Compose, cli, deploy, cloud
---

# Compose - Azure Container Instances mapping

This document outlines the conversion of an application defined in a Compose file to ACI objects.
At a high-level, each Compose deployment is mapped to a single ACI container group.
Each service is mapped to a container in the container group. The Docker ACI integration does not allow scaling of services.

## Compose fields mapping

The table below lists supported Compose file fields and their ACI counterparts.

__Legend:__

- __✓:__ Implemented
- __n:__ Not yet implemented
- __x:__ Not applicable / no available conversion

| Keys                           |Map|  Notes                                                       |
|--------------------------------|---|--------------------------------------------------------------|
| __Service__                    | ✓ |
| service.service.build          | x |  Ignored. No image build support on ACI.
| service.cap_add, cap_drop      | x |
| service.command                | ✓ |  Override container Command. On ACI, specifying `command` will override the image command and entrypoint, if the image has an command or entrypoint defined |
| service.configs                | x |
| service.cgroup_parent          | x |
| service.container_name         | x |  Service name is used as container name on ACI.
| service.credential_spec        | x |
| service.deploy                 | ✓ |
| service.deploy.endpoint_mode   | x |
| service.deploy.mode            | x |
| service.deploy.replicas        | x |  Only one replica is started for each service.
| service.deploy.placement       | x |
| service.deploy.update_config   | x |
| service.deploy.resources       | ✓ |  Restriction: ACI resource limits cannot be greater than the sum of resource reservations for all containers in the container group. Using container limits that are greater than container reservations will cause containers in the same container group to compete with resources.
| service.deploy.restart_policy  | ✓ |  One of: `any`, `none`, `on-failure`. Restriction: All services must have the same restart policy. The entire ACI container group will be restarted if needed.
| service.deploy.labels          | x |  ACI does not have container-level labels.
| service.devices                | x |
| service.depends_on             | x |
| service.dns                    | x |
| service.dns_search             | x |
| service.domainname             | ✓ |  Mapped to ACI DNSLabelName. Restriction: all services must specify the same `domainname`, if specified. `domainname` must be unique globally in <region>.azurecontainer.io
| service.tmpfs                  | x |
| service.entrypoint             | x |  ACI only supports overriding the container command.
| service.env_file               | ✓ |
| service.environment            | ✓ |
| service.expose                 | x |
| service.extends                | x |
| service.external_links         | x |
| service.extra_hosts            | x |
| service.group_add              | x |
| service.healthcheck            | ✓ |
| service.hostname               | x |
| service.image                  | ✓ |  Private images will be accessible if the user is logged into the corresponding registry at deploy time. Users will be automatically logged in to Azure Container Registry using their Azure login if possible.
| service.isolation              | x |
| service.labels                 | x |  ACI does not have container-level labels.
| service.links                  | x |
| service.logging                | x |
| service.network_mode           | x |
| service.networks               | x |  Communication between services is implemented by defining mapping for each service in the shared `/etc/hosts` file of the container group. Each service can resolve names for other services and the resulting network calls will be redirected to `localhost`.
| service.pid                    | x |
| service.ports                  | ✓ |  Only symmetrical port mapping is supported in ACI. See [Exposing ports](#exposing-ports).
| service.secrets                | ✓ |  See [Secrets](#secrets).
| service.security_opt           | x |
| service.stop_grace_period      | x |
| service.stop_signal            | x |
| service.sysctls                | x |
| service.ulimits                | x |
| service.userns_mode            | x |
| service.volumes                | ✓ |  Mapped to AZure File Shares. See [Persistent volumes](#persistent-volumes).
| service.restart                | x |  Replaced by service.deployment.restart_policy
|                                |   |
| __Volume__                     | x |
| driver                         | ✓ |  See [Persistent volumes](#persistent-volumes).
| driver_opts                    | ✓ |
| external                       | x |
| labels                         | x |
|                                |   |
| __Secret__                     | x |
| TBD                            | x |
|                                |   |
| __Config__                     | x |
| TBD                            | x |
|                                |   |


## Logs

Container logs can be obtained for each container with `docker logs <CONTAINER>`.
The Docker ACI integration does not currently support aggregated logs for containers in a Compose application, see https://github.com/docker/compose-cli/issues/803.

## Exposing ports

When one or more services expose ports, the entire ACI container group will be exposed and will get a public IP allocated.
As all services are mapped to containers in the same container group, only one service cannot expose a given port number.
[ACI does not support port mapping](https://feedback.azure.com/forums/602224-azure-container-instances/suggestions/34082284-support-for-port-mapping), so the source and target ports defined in the Compose file must be the same.

When exposing ports, a service can also specify the service `domainname` field to set a DNS hostname. `domainname` will be used to specify the ACI DNS Label Name, and the ACI container group will be reachable at <domainname>.<region>.azurecontainer.io.
All services specifying a `domainname` must set the same value, as it is applied to the entire container group.
`domainname` must be unique globally in <region>.azurecontainer.io

## Persistent volumes

Docker volumes are mapped to Azure file shares. Only the long Compose volume format is supported meaning that volumes must be defined in the `volume` section.
Volumes are defined with a name, the `driver` field must be set to `azure_file`, and `driver_options` must define the storage account and file share to use for the volume.
A service can then reference the volume by its name, and specify the target path to be mounted in the container.

```yaml
services:
    myservice:
        image: nginx
        volumes:
        - mydata:/mount/testvolumes

volumes:
  mydata:
    driver: azure_file
    driver_opts:
      share_name: myfileshare
      storage_account_name: mystorageaccount
```

The short volume syntax is not allowed for ACI volumes, as it was designed for local path bind mounting when running local containers.
A Compose file can define several volumes, with different Azure file shares or storage accounts.

Credentials for storage accounts will be automatically fetched at deployment time using the Azure login to retrieve the storage account key for each storage account used.

## Secrets

Secrets can be defined in compose files, and will need secret files available at deploy time next to the compose file.
The content of the secret file will be made available inside selected containers, by default under `/run/secrets/<SECRET_NAME>`.
External secrets are not supported with the ACI integration.

```yaml
services:
    nginx:
        image: nginx
        secrets:
          - mysecret1
    db:
        image: mysql
        secrets:
          - mysecret2

secrets:
  mysecret1:
    file: ./my_secret1.txt
  mysecret2:
    file: ./my_secret2.txt
```

The nginx container will have secret1 mounted as `/run/secrets/mysecret1`, the db container will have secret2 mounted as `/run/secrets/mysecret2`

A target can also be specified to set the name of the mounted file or by specifying an absolute path where to mount the secret file

```yaml
services:
    nginx:
        image: nginx
        secrets:
          - source: mysecret1
            target: renamedsecret1.txt
    db:
        image: mysql
        secrets:
          - source: mysecret1
            target: /mnt/dbmount/mysecretonmount1.txt
          - source: mysecret2
            target: /mnt/dbmount/mysecretonmount2.txt

secrets:
  mysecret1:
    file: ./my_secret1.txt
  mysecret2:
    file: ./my_secret2.txt
```

In this example the `nginx` service will have its secret mounted to `/run/secrets/renamedsecret1.txt` and `db` will have 2 files (`mysecretonmount1.txt` and `mysecretonmount2.txt`).
Both of them with be mounted in the same folder (`/mnt/dbmount/`).


**Note:** Relative file paths are not allowed in the target

**Note:** Secret files cannot be mounted in a folder next to other existing files

## Container Resources

CPU and memory reservations and limits can be set in compose.
Resource limits must be greater than reservation. In ACI, setting resource limits different from resource reservation will cause containers in the same container group to compete for resources. Resource limits cannot be greater than the total resource reservation for the container group. (Therefore single containers cannot have resource limits different from resource reservations)

```yaml
services:
  db:
    image: mysql
    deploy:
      resources:
        reservations:
          cpus: '2'
          memory: 2G
        limits:
          cpus: '3'
          memory: 3G
  web:
    image: nginx
    deploy:
      resources:
        reservations:
          cpus: '1.5'
          memory: 1.5G
```

In this example, the db container will be allocated 2 CPUs and 2G of memory. It will be allowed to use up to 3 CPUs and 3G of memory, using some of the resources allocated to the web container.
The web container will have its limits set to the same values as reservations, by default.

## Healthchecks

A health check can be described in the `healthcheck` section of each service. This is translated to a `LivenessProbe` in ACI. If the health check fails then the container is considered unhealthy and terminated.
In order for the container to be restarted automatically, the service needs to have a restart policy other than `none` to be set. Note that the default restart policy if one isn't set is `any`.

```yaml
services:
  web:
    image: nginx
    deploy:
      restart_policy:
        condition: on-failure
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:80"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

**Note:** that the `test` command can be a `string` or an array starting or not by `NONE`, `CMD`, `CMD-SHELL`. In the ACI implementation, these prefixes are ignored.
