## Compose sample application

### Python/Flask application

```
+--------------------+              +------------------+
|                    |              |                  |
|    Python Flask    |  timestamps  |      Redis       |
|    Application     |------------->|                  |
|                    |              |                  |
+--------------------+              +------------------+
```

### Things you'll need to do.

There are a number of places you'll need to fill in information. You can find them with:

```
grep -r '<<<' ./*
./docker-compose.yml:    x-aws-pull_credentials: <<<your arn for your secret you can get with docker ecs secret list>>>
./docker-compose.yml:    image: <<<yourhubname>>>/timestamper
## Walk through
```

### Setup pull credentials for private Docker Hub repositories

You should use a Personal Access Token (PAT) vs your default password. If you have 2FA enabled on your Hub account you will have to create a PAT. You can read more about managing access tokens here: https://docs.docker.com/docker-hub/access-tokens/

```
  docker ecs secret create -d MyKey -u myhubusername -p myhubpat
```

### Create an AWS Docker context and list available contexts

```
docker ecs setup
Enter context name: aws
✔ sandbox.devtools.developer
Enter cluster name:
Enter region: us-west-2
✗ Enter credentials:

docker context ls
NAME                DESCRIPTION                               DOCKER ENDPOINT               KUBERNETES ENDPOINT   ORCHESTRATOR
aws
default *           Current DOCKER_HOST based configuration   unix:///var/run/docker.sock                         swarm
```

### Test locally

```
docker context use default
docker-compose up
open http://localhost:5000
```

### Push images to hub for ecs (ecs cannot see your local image cache)

```
docker-compose push
```

### Switch to ECS context and launch the app

```
docker context use aws
docker ecs compose up
```

### Check out the CLI

```
docker ecs compose ps
docker ecs compose logs
```

### Check out the aws console

- cloud formation
- cloud watch
- security groups
- Load balancers (ELB for this example / ALB if your app only uses 80/443)

### Checkout cloudformation

```
docker ecs compose convert
```

### Stop the meters

```
docker ecs compose down

```

## Using Amazon ECR instead of Docker Hub

[Makefile](Makefile) has an example setup for creating an ECR repository and pushing to it. You'll need to have the AWS CLI installed and your AWS credentials available.

```
make create-ecr
REGISTRY_ID=<from the create above> make build-image
REGISTRY_ID=<from the create above> make push-image-ecr
```

If you want to use this often, you'll likely want to replace `PUT_ECR_REGISTRY_ID_HERE` with the value from above.
