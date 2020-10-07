# Compose - Azure Container Instances mapping

This document outlines the conversion of an application defined in a Compose file to ACI objects.
At a high-level, each Compose deployment is mapped to a single ACI container group. 
Each service is mapped to a container in the container group. The Docker ACI integration provides does not allow scaling of services.

## Compose fields mapping

The table below lists supported Compose file fields and their ACI counterparts.

__Legend:__

- __✓:__ Implemented
- __n:__ Not yet implemented
- __x:__ Not applicable / no available conversion

| Keys                           | Map|  Notes                                                       |
|--------------------------------|----|--------------------------------------------------------------|
| __Service__                    | ✓  |                                                              |
| service.service.build          | x  |  Ignored. No image build support on ACI.                              |
| service.cap_add, cap_drop      | x  |                                                              |
| service.command                | ✓  |  Override container Command. on ACI, specifying command will currently also override image entrypoint if the image has an Entrypoint defined |
| service.configs                | n  |                                                              |
| service.cgroup_parent          | x  |  
| service.container_name         | x  |  Service name is used as container name on ACI.               |
| service.credential_spec        | x  |                                                              |
| service.deploy                 | ✓  |                                                              |
| service.deploy.endpoint_mode   | x  |                                                              |
| service.deploy.mode            | x  |                                                              |
| service.deploy.replicas        | x  |  Only one replica is started for each service. | 
| service.deploy.placement       | x  |  
| service.deploy.update_config   | x  |  
| service.deploy.resources       | ✓  |  Restriction: ACI Resource limits cannot be greater than the sum of resource requests for all containers in the container group. This will allow one containers in the same container group to compete with resources. 
| service.deploy.restart_policy  | ✓  |  One of: `any`, `none`, `on-failure`. Restriction: All services must have the ame restart policy. The entire ACI container group will be restarted if needed. | 
| service.deploy.labels          | x  |  ACI does not have container-level labels. | 
| service.devices                | x  | 
| service.depends_on             | x  | 
| service.dns                    | x  | 
| service.dns_search             | x  | 
| service.domainname             | ✓  |  Mapped to ACI DNSLabelName. Restriction: all services must specify the same `domainname`, if specified. |
| service.tmpfs                  | x  |  
| service.entrypoint             | x  |  ACI only supports overriding the container command. |
| service.env_file               | ✓  |  
| service.environment            | ✓  |  
| service.expose                 | x  |   
| service.extends                | x  |  
| service.external_links         | x  |  
| service.extra_hosts            | x  |  
| service.group_add              | x  |  
| service.healthcheck            | x  |  
| service.hostname               | x  |  
| service.image                  | ✓  |  Private images will be accessible if the user is logged into the corresponding registry at deploy time. Users will be automatically logged in to Azure Container Registry using their Azure login if possible. |
| service.isolation              | x  |  
| service.labels                 | x  |  ACI does not have container-level labels. |
| service.links                  | x  |  
| service.logging                | x  |  
| service.network_mode           | x  |  
| service.networks               | x  |  Communication between services is implemented by defining mapping for each service in the shared `/etc/hosts` file of the container group. Each service can resolve names for other services and the resulting network calls will be redirected to `localhost`. |
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
| service.restart                | x  |  Replaced by service.deployment.restart_policy                |
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
              

## Exposing ports

When one or more services expose ports, the entire ACI container group will be exposed and will get a public IP allocated.   
As all services are mapped to containers in the same container group, only one service cannot expose a given port number.
[ACI does not support port mapping](https://feedback.azure.com/forums/602224-azure-container-instances/suggestions/34082284-support-for-port-mapping), so the source and target ports defined in the Compose file must be the same.

When exposing ports, a service can also specify the service `domainname` field to set a DNS hostname. `domainname` will be used to specify the ACI DNS Label Name, and the ACI container group will be reachable at <domainname>.<region>.azurecontainer.io.
All services specifying a `domainname` must set the same value, as it is applied to the entire container group.

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
