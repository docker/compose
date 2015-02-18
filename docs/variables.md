---
layout: default
title: docker-compose.yml reference variables
page_title: docker-compose.yml reference variables
page_description: docker-compose.yml reference variables
page_keywords: fig, composition, compose, docker
---

# docker-compose.yml reference variables

When you scale a service defined in  `docker-compose.yml`, container names
are automatically generated with the pattern PROJECTNAME_SERVICE_NUM.

To allow conflicting hostnames, you can use the %%id%% placeholder, that
will be replaced with the container number.

This approach avoids messing with environment and all its security issues
and perks (eg. involving bash variable expansion)

```
test:
  image: busybox:latest
  hostname: box-%%id%%
```

The placeholders: %%id%% and %%hostname%% are available to the 'command' options.
You can then:

```
mysql:
  image: ioggstream/mysql
  hostname: db-%%id%%
  ...
  command: mysqld --server-id=%%id%% --relay-log=%%hostname%%-relay-log

```

To avoid conflicts in numeric fields (eg: server-id) you can just prepend a
large integer (eg. 100)

```
master:
  image: ioggstream/mysql
  hostname: master-%%id%%
  ...
  command: mysqld --server-id=100%%id%% --relay-log=%%hostname%%-relay-log

slave:
  image: ioggstream/mysql
  hostname: slave-%%id%%
  ...
  command: mysqld --server-id=%%id%% --relay-log=%%hostname%%-relay-log


```


### Running

When running with:

```
#docker-compose scale master=1 slave=3
```

You get

```
#docker inspect -f '{{.Name}} {{.Config.Hostname}}' $(docker ps -q) | column -t
/db_slave_3               slave-3
/db_slave_2               slave-2
/db_slave_1               slave-1
/db_master_1              master-1
```

