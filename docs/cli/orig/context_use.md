---
title: "context use"
description: "The context use command description and usage"
keywords: "context, use"
---

<!-- This file is maintained within the docker/cli GitHub
     repository at https://github.com/docker/cli/. Make all
     pull requests against that repo. If you see this file in
     another repository, consider it read-only there, as it will
     periodically be overwritten by the definitive file. Pull
     requests which include edits to this file in other repositories
     will be rejected.
-->

# context use

```markdown
Usage:  docker context use CONTEXT

Set the current docker context
```

## Description
Set the default context to use, when `DOCKER_HOST`, `DOCKER_CONTEXT` environment variables and `--host`, `--context` global options are not set.
To disable usage of contexts, you can use the special `default` context.