---
layout: default
title: Fig environment variables reference
---

Environment variables reference
===============================

**Note:** Environment variables are no longer the recommended method for connecting to linked services. Instead, you should use the link name (by default, the name of the linked service) as the hostname to connect to. See the [fig.yml documentation](yml.html#links) for details.

Fig uses [Docker links] to expose services' containers to one another. Each linked container injects a set of environment variables, each of which begins with the uppercase name of the container.

To see what environment variables are available to a service, run `fig run SERVICE env`.

<b><i>name</i>\_PORT</b><br>
Full URL, e.g. `DB_PORT=tcp://172.17.0.5:5432`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i></b><br>
Full URL, e.g. `DB_PORT_5432_TCP=tcp://172.17.0.5:5432`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i>\_ADDR</b><br>
Container's IP address, e.g. `DB_PORT_5432_TCP_ADDR=172.17.0.5`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i>\_PORT</b><br>
Exposed port number, e.g. `DB_PORT_5432_TCP_PORT=5432`

<b><i>name</i>\_PORT\_<i>num</i>\_<i>protocol</i>\_PROTO</b><br>
Protocol (tcp or udp), e.g. `DB_PORT_5432_TCP_PROTO=tcp`

<b><i>name</i>\_NAME</b><br>
Fully qualified container name, e.g. `DB_1_NAME=/myapp_web_1/myapp_db_1`

[Docker links]: http://docs.docker.io/en/latest/use/port_redirection/#linking-a-container
