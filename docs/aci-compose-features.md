# Compose - Azure Container Instances mapping

This document outlines mapping between `docker-compose.yaml` files and corresponding ACI equivalents.
At a high-level, each Compose deployment is mapped to a single ACI container group. 
Each service is mapped to a container in the container group, the Docker ACI integration provides no scaling of services. 

## Compose fields mapping

Table bellow list supported Compose file fields with equivalent ACI when supported.

__Glossary:__

- __✓:__ Converts
- __n:__ Not yet implemented
- __x:__ Not applicable / no 1-1 conversion

| Keys                           | Map|  Notes                                                       |
|--------------------------------|----|--------------------------------------------------------------|
| __Service__                    | ✓  |                                                              |
| service.service.build          | x  |  Ignored. no image build support on ACI.                              |
| service.cap_add, cap_drop      | x  |                                                              |
| service.command                | ✓  |  Override container Command. on ACI, specifying command will currently also override image entrypoint if the image has an Entrypoint defined |
| service.configs                | n  |                                                              |
| service.cgroup_parent          | x  |  
| service.container_name         | x  |  Service name is used as container name in ACI               |
| service.credential_spec        | x  |                                                              |
| service.deploy                 | ✓  |                                                              |
| service.deploy.endpoint_mode   | x  |                                                              |
| service.deploy.mode            | x  |                                                              |
| service.deploy.replicas        | x  |  Only one replica is started for each service with the docker-ACI integration. | 
| service.deploy.placement       | x  |  
| service.deploy.update_config   | x  |  
| service.deploy.resources       | ✓  |  Restriction: ACI Resource limits cannot be greater than the sum of resource requests for all containers in the container group. This will allow one containers in the same container group to compete with resources. 
| service.deploy.restart_policy  | ✓  |  One of: any - none - on-failure. Restriction: All services must have the ame restart policy. The entire ACI container group will be restarted if needed. | 
| service.deploy.labels          | x  |  ACI does not have container-level labels. In addition, Azure tag are too restrictive to support Docker container labels.| 
| service.devices                | x  | 
| service.depends_on             | x  | 
| service.dns                    | x  | 
| service.dns_search             | x  | 
| service.domainname             | ✓  |  Mapped to ACI DNSLabelName. Restriction: all services must specify the same domainname, if specified.
| service.tmpfs                  | x  |  
| service.entrypoint             | x  |  ACI allows to override Command, but not Entrypoint. 
| service.env_file               | ✓  |  
| service.environment            | ✓  |  
| service.expose                 | x  |   
| service.extends                | x  |  
| service.external_links         | x  |  
| service.extra_hosts            | x  |  
| service.group_add              | x  |  
| service.healthcheck            | x  |  
| service.hostname               | x  |  
| service.image                  | ✓  |  Private images will be accessible if the user is logged in against the corresponding registry at deploy time. Users will be automatically logged in to Azure Container Registry from Azure login if possible.
| service.isolation              | x  |  
| service.labels                 | x  |  ACI does not have container-level labels. In addition, Azure tag are too restrictive to support Docker container labels.  |
| service.links                  | x  |  
| service.logging                | x  |  
| service.network_mode           | x  |  
| service.networks               | x  |  Communication between services is supported by defining mapping for each service in `/etc/hosts` in the container group. Each service can resolve names for other services and the resulting network calls will be redirected to `localhost`.
| service.pid                    | x  |  
| service.ports                  | ✓  |  
| service.secrets                | ✓  |    
| service.security_opt           | x  |  
| service.stop_grace_period      | x  |  
| service.stop_signal            | x  |  
| service.sysctls                | x  |  
| service.ulimits                | x  |  
| service.userns_mode            | x  |  
| service.volumes                | ✓  |  Mapped to AZure File Shares. See #persistent_volumes
| service.restart                | x  |  Replaced by service.deployment.restrt_policy                |
|                                |    |                                                              |
| __Volume__                     | x  |                                                              |
| driver                         | ✓  |                                                              |
| driver_opts                    | ✓  |                                                              |
| external                       | x  |                                                              |
| labels                         | x  |                                                              |
|                                |    |                                                              |
| __Secret__                     | x  |                                                              |
| TBD                            | x  |                                                              |
|                                |    |                                                              |
| __Config__                     | x  |                                                              |
| TBD                            | x  |                                                              |
|                                |    |                                                              |
              

## Port Exposure

When one of the service exposes ports, the entire ACI container group will be exposed and will get a public IP allocated.   
Since all services are mapped to containers in the same container group, several services cannot expose the same port number. 
[ACI does not support port mapping](https://feedback.azure.com/forums/602224-azure-container-instances/suggestions/34082284-support-for-port-mapping), so source and target port defined in the Compose file must be identical.

When exposing ports, a service can also specify the service `domainname` field to set a DNS hostname. `domainname` will be used to specify ACI DNS Label Name, and the ACI container group will be reachable at <domainname>.region.azurecontainer.io.
All services specifying a domainname must set the same value, applied to the entire container group. 

## Persistent Volumes

Docker volumes are mapped to Azure File Shares. The compose file must define volumes in the `volume` section in order to be able to use them for one or several services. 
Volumes are defined with a name, the `driver` field must be set to `azure_file`, and `driver_options` must define the storage account and fileshare to use for the volume. 
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

Short volume syntax is not allowed for ACI volumes, as it wa designed for local path when running local containers.  
A Compose file can defined several volumes, with different Azure file shares or storage accounts.

Credentials for storage account will be automatically fetched at deployment time, using the Azure login to retrive storage account key for each storage account used. 