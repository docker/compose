<!--[metadata]>
+++
title = "bundle"
description = "Create a distributed application bundle from the Compose file."
keywords = ["fig, composition, compose, docker, orchestration, cli,  bundle"]
[menu.main]
identifier="bundle.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# bundle

```
Usage: bundle [options]

Options:
    --push-images              Automatically push images for any services
                               which have a `build` option specified.

    -o, --output PATH          Path to write the bundle file to.
                               Defaults to "<project name>.dab".
```

Generate a Distributed Application Bundle (DAB) from the Compose file.

Images must have digests stored, which requires interaction with a
Docker registry. If digests aren't stored for all images, you can fetch
them with `docker-compose pull` or `docker-compose push`. To push images
automatically when bundling, pass `--push-images`. Only services with
a `build` option specified will have their images pushed.
