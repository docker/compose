Getting Started with Docker AWS ECS Plugin Beta on Linux
--------------------------------------------------------

The beta release of [AWS ECS](https://aws.amazon.com/ecs/) support for the
Docker CLI is shipped as a CLI plugin. Later releases will be included as part
of the Docker CLI.

This plugin is included as part of Docker Desktop on Windows and macOS but on
Linux it needs to be installed manually.

## Prerequisites

* [Docker 19.03 or later](https://docs.docker.com/get-docker/)

## Step by step install

### Download

You can download the Docker ECS plugin from this repository using the following
command:

<!-- FIXME(chris-crone): get real download link -->
```console
$ curl -L http://xxx | tar xzf -
```

You will then need to make it executable:

```console
$ chmod +x docker-ecs
```

### Plugin install

In order for the Docker CLI to use the downloaded plugin, you will need to move
it to the right place:

```console
$ mkdir -p /usr/local/lib/docker/cli-plugins

$ mv docker-ecs /usr/local/lib/docker/cli-plugins/
```

You can put the CLI plugin into any of the following directories:

* `/usr/local/lib/docker/cli-plugins`
* `/usr/local/libexec/docker/cli-plugins`
* `/usr/lib/docker/cli-plugins`
* `/usr/libexec/docker/cli-plugins`

Finally you need to enable the experimental features on the CLI. This can be
done by setting the environment variable `DOCKER_CLI_EXPERIMENTAL=enabled` or by
setting `experimental` to `"enabled"` in your Docker config found at
`~/.docker/config.json`:

```console
$ export DOCKER_CLI_EXPERIMENTAL=enabled

$ DOCKER_CLI_EXPERIMENTAL=enabled docker help

$ cat ~/.docker/config.json
{
  "experimental" : "enabled",
  "auths" : {
    "https://index.docker.io/v1/" : {

    }
  }
}
```

To verify the CLI plugin installation, you can check that it appears in the CLI
help output or by outputting the plugin version:

```console
$ docker help | grep ecs
  ecs*        Docker ECS (Docker Inc., 0.0.1)

$ docker ecs version
Docker ECS plugin 0.0.1
```

<!-- FIXME(chris-crone): Link to ECS docs -->
You are now ready to [start deploying to ECS](http://xxx)
