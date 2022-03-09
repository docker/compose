# docker compose push

<!---MARKER_GEN_START-->
Push service images

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `--ignore-push-failures` |  |  | Push what it can and ignores images with push failures |


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
