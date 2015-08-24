<!--[metadata]>
+++
title = "port"
description = "Prints the public port for a port binding.s"
keywords = ["fig, composition, compose, docker, orchestration, cli,  port"]
[menu.main]
identifier="port.compose"
parent = "smn_compose_cli"
+++
<![end-metadata]-->

# port

```
Usage: port [options] SERVICE PRIVATE_PORT

Options:
--protocol=proto  tcp or udp [default: tcp]
--index=index     index of the container if there are multiple
                  instances of a service [default: 1]
```

Prints the public port for a port binding.
