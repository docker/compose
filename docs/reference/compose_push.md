# docker compose push

<!---MARKER_GEN_START-->
Pushes images for services to their respective registry/repository.

The following assumptions are made:
- You are pushing an image you have built locally
- You have access to the build key

Examples

```yaml
services:
  service1:
    build: .
    image: localhost:5000/yourimage  ## goes to local registry

  service2:
    build: .
    image: your-dockerid/yourimage  ## goes to your repository on Docker Hub
```

### Options

| Name                     | Type   | Default | Description                                            |
|:-------------------------|:-------|:--------|:-------------------------------------------------------|
| `--dry-run`              | `bool` |         | Execute command in dry run mode                        |
| `--ignore-push-failures` | `bool` |         | Push what it can and ignores images with push failures |
| `--include-deps`         | `bool` |         | Also push images of services declared as dependencies  |
| `-q`, `--quiet`          | `bool` |         | Push without printing progress information             |


<!---MARKER_GEN_END-->

## Description

Pushes images for services to their respective registry/repository.

The following assumptions are made:
- You are pushing an image you have built locally
- You have access to the build key

Examples

```yaml
services:
  service1:
    build: .
    image: localhost:5000/yourimage  ## goes to local registry

  service2:
    build: .
    image: your-dockerid/yourimage  ## goes to your repository on Docker Hub
```
