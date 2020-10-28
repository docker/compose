# Features

## Login

You can log into Azure with the `docker login azure` command. This will open a Web browser to complete the login process. If the Docker CLI cannot open a browser, it will fall back to the [Azure device code flow](https://docs.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-device-code) and let you connect yourself.
Note that the [Azure command line](https://docs.microsoft.com/en-us/cli/azure/) login is separated from the Docker CLI Azure login.

## Docker contexts

The Docker ACI integration is performed through the use of Docker contexts. You can create a context of type ACI, and then using this context you will be able to run classic Docker commands (like `docker run`) against ACI.
Creating an ACI context requires an Azure subscription, a resource group and a region. 

Before creating a context, you will need to `docker login azure` first so that the Docker CLI can find your Azure subscription.

To create an ACI context, `docker context create aci` will prompt you for the relevant information, with the option to either create a new resource group or use an existing one.
With a context created, you can either issue single commands against ACI, specifying the context name in the command line, like `docker --context acicontext run nginx`, or switch context so that all subsequent commands will be issued to ACI: `docker context use acicontext`.

**Note:** You can have multiple ACI contexts associated each with different resource groups. This can be useful as it will act as namespaces for your containers. Actions on your containers will be restricted to your current context and resource group.

## Single container

You can start a single container on ACI with the `docker run` command. 
The container is started in a container group, in the region and resource group associated with the ACI context.

You can then list existing containers with `docker ps`. This will show the status of the container, if it exposes ports, and the container's public IP address or domain name.
You can view a container's logs with the `docker logs` command.

Running containers can be stopped with `docker stop` (`docker kill` will have the same effect). Stopped containers can be restarted with `docker start`.
 
**Note:** The semantics of restarting a container on ACI are different to those when using a moby type context for local development. On ACI, the container will be reset to its initial state and started on a new node. This includes the container's filesystem so all state that is not stored in a volume will be lost on restart.

Containers can be removed using `docker rm`. Removing a running container will require using the `--force` flag, or stopping the container with `docker stop` before removing.   

## Compose applications

You can start Compose applications with the `docker compose up` command.
All containers in the same Compose application are started in the same container group. Service discovery between the containers works using the service name specified in the Compose file.
Name resolution is achieved by writing service names in the `/etc/hosts` file that is shared automatically by all containers in the container group.

Containers started as part of Compose applications will be displayed along with single containers when using `docker ps`. 
Their container ID will be of the format: `<COMPOSE-PROJECT>_<SERVICE>`. 
These containers cannot be stopped, started or removed independently since they are all part of the same ACI container group. You can view each container's logs with `docker logs`.
You can list deployed Compose applications with `docker compose ls`. This will list only compose applications, not single containers started with `docker run`.
You can remove a Compose application with `docker compose down`.

## Exposing ports

Single containers and Compose applications can optionally expose ports. For single containers this is done using the `--publish` flag of the `docker run` command and for Compose applications this is done in the Compose file.
 
**Note:** ACI does not allow port mapping (i.e.: changing port number while exposing port), so the source and target ports must be the same when deploying to ACI.

**Note:** All containers in the same Compose application are deployed in the same ACI container group. Containers in the same Compose application cannot expose the same port when deployed to ACI.

By default, when exposing ports for your application, a random public IP address is associated with the container group supporting the deployed application (single container or Compose application).
This IP address can be obtained when listing containers with `docker ps` or using `docker inspect`.    

### DNS label name

In addition to exposing ports on a random IP address, you can specify a DNS label name to expose your application on an FQDN of the form: `<NAME>.region.azurecontainer.io`.
This name can be set with the `--domain` flag when performing a `docker run` or using the `domain` field in the Compose file when performing a `docker compose up`.
**Note:** The domain of a Compose application can only be set once, if you specify `domain` for several services, the value must be identical.

## Volumes

Single containers and Compose applications can use volumes. In ACI, volumes are implemented as Azure file shares in Azure storage accounts.
To use a volume when running a single container, specify a storage account and file share name like this: 

```console
docker run -v <STORAGE-ACCOUNT>/<FILE-SHARE>:/target/path[:ro]
```
  
To specify more than one volume, just use the `-v` option as many times as required.
Compose application volumes are specified in the _volumes_ section of the Compose file and attached to a service as follows:

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

**Note:** The volume short syntax in Compose files cannot be used as it is aimed at volume definition for local bind mounts. Using the volume driver and driver option syntax in Compose files makes the volume definition a lot more clear.

In both cases, when deploying the containers, the Docker CLI will use your Azure login to fetch the key to the storage account, and provide this key with the container deployment information, so that the container can access the volume.
Volumes can be used from any file share in any storage account you have access to with your Azure login. You can specify `rw` (read/write) or `ro` (read only) when mounting the volume (`rw` is the default).

ACI volumes can be managed with the `volume` subcommand. This includes `docker volume ls`, `docker volume create`, and `docker volume rm`. 
Creating a volume requires a storage account name and a volume name. The storage account is created using default options if it does not exist, using the region associated with your ACI context.  
If the storage account name already exists (and you have access to it), a file share is created with default values, using this existing storage account. Volume creation fails if the file share already exists.

Listing volumes will list all file shares you have access using your Azure login, in any storage account. You can use existing file shares as volumes, even if they have not been created through the Docker CLI.

When using `docker volume rm`, the command will remove the specified file share from the specified storage account. If this storage account has no more file shares, and if the storage account was created by the Docker CLI, then the storage account will also be deleted.

## Resource usage definition

You can specify CPU and memory limits for your containers.
For single containers, use `docker run --cpu 2 --memory 2G`. 
In Compose files, you can use the service resource limits:

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

In this example, the _redis_ service is constrained to use no more than 50M of memory (50 MB) and 0.50 (50% of a single core) of available processing time (CPU).

## Environment variables

When using `docker run`, environment variables can be passed to ACI containers using the `--env` flag.
Form compose applications, environment variables can be specified in the compose file with the `environment` or `env-file` service field, or with the `--environment` command line flag.

## Private Docker Hub images and using the Azure Container Registry

You can deploy private images to ACI that are hosted by any container registry. You need to `docker login` to the relevant registry before running `docker run` or `docker compose up`. The Docker CLI will fetch your registry login for the deployed images and send the credentials along with the image deployment information to ACI.
In the case of the Azure Container Registry, the command line will try to automatically log you into ACR from your Azure login. You don't need to manually login to the ACR registry first, if your Azure login has access to the ACR.
