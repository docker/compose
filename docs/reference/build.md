<!--[metadata]>
+++
title = "build"
description = "build"
keywords = ["fig, composition, compose, docker, orchestration, cli,  build"]
[menu.main]
identifier="build.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# build

```
Usage: build [options] [SERVICE...]

Options:
--no-cache  Do not use cache when building the image.
```

Services are built once and then tagged as `project_service`, e.g.,
`composetest_db`. If you change a service's Dockerfile or the contents of its
build directory, run `docker-compose build` to rebuild it.
