## Compose sample application

This sample application was demoed as part of the AWS Cloud Containers
Conference on 2020-07-09. It has been tested on Linux and macOS.

Note that `$` is used to denote commands in blocks where the command and its
output are included.

### Python/Flask application

```
+--------------------+              +------------------+
|                    |              |                  |
|    Python Flask    |  timestamps  |      Redis       |
|    Application     |------------->|                  |
|                    |              |                  |
+--------------------+              +------------------+
```

### Things you'll need to do

There are a number of places you'll need to fill in information.
You can find them with:

```console
$ grep -r '<<<' ./*
./docker-compose.yml:    x-aws-pull_credentials: <<<your arn for your secret you can get with docker ecs secret list>>>
./docker-compose.yml:    image: <<<your docker hub user name>>>/timestamper
## Walk through
```

### Setup pull credentials for private Docker Hub repositories

You should use a Personal Access Token (PAT) rather than your account password.
If you have 2FA enabled on your Hub account you will need to create a PAT.
You can read more about managing access tokens here:
https://docs.docker.com/docker-hub/access-tokens/


You can then create `DockerHubToken` secret on [AWS Secret Manager](https://aws.amazon.com/secrets-manager/) using following command

```console
docker ecs secret create -d MyKey -u myhubusername -p myhubpat DockerHubToken
```

### Create an AWS Docker context and list available contexts

To initialize the Docker ECS integration, you will need to run the `setup`
command. This will create a Docker context that works with AWS ECS.

```console
$ docker ecs setup
Enter context name: aws
✔ sandbox.devtools.developer
Enter cluster name:
Enter region: us-west-2
✗ Enter credentials:
```

You can verify that the context was created by listing your Docker contexts:

```console
$ docker context ls
NAME                DESCRIPTION                               DOCKER ENDPOINT               KUBERNETES ENDPOINT   ORCHESTRATOR
aws
default *           Current DOCKER_HOST based configuration   unix:///var/run/docker.sock                         swarm
```

### Test locally

The first step is to test your application works locally. To do this, you will
need to switch to using the default local context so that you are targeting your
local machine.

```console
docker context use default
```

You can then run the application using `docker-compose`:

```console
docker-compose up
```

Once the application has started, you can navigate to http://localhost:5000
using your Web browser using the following command:

```console
open http://localhost:5000
```

### Push images to Docker Hub for ECS (ECS cannot see your local image cache)

In order to run your application in the cloud, you will need your container
images to be in a registry. You can push them from your local machine using:

```console
docker-compose push
```

You can verify that this command pushed to the Docker Hub by
[logging in](https://hub.docker.com) and looking for the `timestamper`
repository under your user name.

### Switch to ECS context and launch the app

Now that you've tested the application works locally and that you've pushed the
container images to the Docker Hub, you can switch to using the `aws` context
you created earlier.

```console
docker context use aws
```

Running the application on ECS is then as simple as doing a `compose up`:

```console
docker ecs compose up
```

### Check out the CLI

Once the application is running in ECS, you can list the running containers with
the `ps` command. Note that you will need to run this from the directory where
you Compose file is.

```console
docker ecs compose ps
```

You can also read the application logs using `compose logs`:

```console
docker ecs compose logs
```

### Check out the AWS console

You can see all the AWS components created for your running application in the
AWS console. There you will find:

- CloudFormation being used to manage all the infrastructure
- CloudWatch for logs
- Security Groups for network policies
- Load balancers (ELB for this example / ALB if your app only uses 80/443)

### Checkout CloudFormation

The ECS Docker CLI integration has the ability to output the CloudFormation
template used to create the application in the `compose convert` command. You
can see this by running:

```console
docker ecs compose convert
```

### Stop the meters

To shut down your application, you simply need to run:

```console
docker ecs compose down
```

## Using Amazon ECR instead of Docker Hub

If you'd like to use AWS ECR instead of Docker Hub, the [Makefile](Makefile) has
an example setup for creating an ECR repository and pushing to it.
You'll need to have the AWS CLI installed and your AWS credentials available.

```console
make create-ecr
REGISTRY_ID=<from the create above> make build-image
REGISTRY_ID=<from the create above> make push-image-ecr
```

Note that you will need to change the name of the image in the
[Compose file](docker-compose.yml).

If you want to use this often, you'll likely want to replace
`PUT_ECR_REGISTRY_ID_HERE` with the value from above.
