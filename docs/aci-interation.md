# Features

## Login

You can login into Azure with `docker login azure`. This will prompt for user info in a browser. If the docker CLI cannot open a browser, it will switch back to [Azure device code flow](https://docs.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-device-code) and let you connect yourself.
Note that the login form the [Azure command line](https://docs.microsoft.com/en-us/cli/azure/) is separated from the Docker CLI azure login.

## Docker contexts

The Docker ACI integration is performed through the use of Docker contexts. You can create an ACI context, and the using this context you will be able to run classic Docker commands agains ACI. 
Creating a context will require an Azure subscription, a resource group and a region. 

You need to `docker login azure` first, so that the Docker CLI can find your azure subscription based on your login. 

To create an ACI context, `docker context create aci` will prompt you for the relevant information, and either create a new resource group or use an existing one.
With a context created, you can either issue single commands against ACI, specifying the context name in the command line, like `docker --context acicontext run nginx`, or switch context so that all subsequent commands will be issued to ACI: `docker context use acicontext`.

**Note** You can have several ACI context, associated with several resource groups. This can be useful as it will act as namespaces for your containers. Actions on your containers will be restricted to your current context and resource group.

## Single container

You can start a single container on ACI with the `docker run` command. 
The container is started on a container group, in the region and resource group associated with the ACI context.

You can then list existing containers with `docker ps`. This will show the status of the containers, if they expose ports, and where you can connect to the running application.
You can view each container logs witth `docker logs`.

Running containers can be stopped with `docker stop` (`docker kill` will have the same effect), restarted with `docker start`.
 
**Note** The semantics of `docker start` is different from local development, since the container will be restarted from scratch in ACI, on a new node. All previous container state that is not stored in a volume is lost.

Containers can be removed totally with `docker rm`. Removing running containers will require using the `--force` flag, or stopping the containers before removing them.   

## Compose applications

You can start single or multi-container compose applications with the `docker compose up` command.
All containers in the same compose application are started in the same container group. They can see each other by compose service name. 
Name resolution is archieved by writing service names in the /etc/hosts file ; this file is shared automatically in the container group.

Containers started as part of compose applications will be displayed along with single containers when using `docker ps`. 
Their container ID will be formatted like `<COMPOSE-APP>_<SERVICE>`. 
These containers cannot be stopped, started or removed independently since they are all part of the same ACI container group. You can view each container logs with `docker logs`.
You can list deployed compose applications with `docker compose ls`. This will list only compose applications, not single containers started with `docker run`. 
You can remove a compose application with `docker compose down`.

## Port exposure

Single containers and compose applications can expose one or several ports, using either standard port definition in the compose file, or `-p 80:80` format in the `docker run` command.
 
**Note** ACI does not allow port mapping (ie. changing port number while exposing port), so the source and target ports must be the same when deploying to ACI.

By default, when exposing ports for your application, a random public IP address is associated with the container group supporting the deployed application (single container or compose application). 
This IP address can be obtained when listing containers with `docker ps` or using `docker inspect`.    

### DSN Label name

In addition to exposing ports on a random IP address, you can specify a DNS Label name to expose your application on a fqdn of the form: `<name>.region.azurecontainer.io`. 
This can be don with the `--domain` flag in either `docker run` or `docker compose up`. You can also use the `domain` field in compose applications. 
Note in this case you can specifiy only one name for the entire compose application, if you specify `domain` for several services, the value must be identical. 

## Volumes

Single containers and compose applications can use volumes. In ACI, volumes are implemented as Azure Fileshare in Azure Storage Accounts.
To use a volume when running a single container, specify storage account and fileshare name like this: 

```
docker run -v <storageaccount>/<fileshare>:/target/path[:ro]
```
  
You can use one or more volumes, just use the `-v` option as many times as required.  
In a compose application, you will defined volumes like this : 

```yaml
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

The volume short syntax in compose files cannot be used, as it is aimed at volume definition with local folders. Using the volume driver and driver option syntax in compose files makes the volume definition a lot more clear. 

In both cases, when deploying the containers, the Docker cli will use your azure login to fetch the key to the storage account, and provide this key with the container deployment information, so that the container can access the volume.
Volumes can be used from any fileshare in any storage account you have access to with your Azure login. You can specify `rw` or `ro` when mounting the volume (read/write by default).

ACI Volumes can be managed with commands `docker volume ls`, `docker volume create` and `docker volume rm`. 
Creating a volume requires a storage account name and a volume name. The storage account is created with default options if it does not exist, using ther region associated with your ACI context.  
If the storage account name already exists (and you have access to it), a fileshare is created with default values, using this existing storage account. Volume creation fails if the fileshare also already exists.

Listing volumes will list all fileshares you have access from your azure login, in any storage account. You can use existing fileshares as volumes, even if they have not been created throug the Docker CLI. 

When using `docker volume rm`, the command will remove the specified fileshare from the specified storage account. If this storage account has no more fileshares, and if the storage account has been created by the DOcker CLI, then the storage account will also be deleted.   

## Resource usage definition

You can specify resources to be used by your containes, setting CPU and memory limits. 
For single containers, use `docker run --cpu 2 --memory 2G`. 
In compose files, you can use the service resource limits 

```yaml
services:
  redis:
    image: redis:alpine
    deploy:
      resources:
        limits:
          cpus: '0.50'
          memory: 50M
```

## Environment variables

Environment variables can be passed to ACI containers through the `docker run --env` flag.  
Form compose applications, environment variables can be specified with the `--env-file` flag, or in the compose file with the `environment` service field. 

## Private images and Azure Container Registry

You can deploy private images to ACI, from any container registry. You need to `docker login` to the relevant registry before running `docker run` or `docker compose up` against ACI. the docker CLI will fetch your registry login for the deployed images and send the credential along with the image deployment information to ACI. 
In the case of Azure Container Registry, the command line will try to automatically log you in the ACR registry from your azure login, therefore you don't need to manually login to the ACR registry first, iy you azure login has access to the ACR.  
