# containerd

Containerd is a daemon to control runC, built for performance and density. 
Containerd leverages runC advanced features such as seccomp and user namespace support as well
as checkpoint and restore for cloning and live migration of containers.

#### Status 

*alpha*

What does alpha, beta, etc mean?
* alpha - not feature complete
* beta - feature complete but needs testing
* prod ready - read for production

## Docs

For more documentation on various subjects refer to the `/docs` directory in this repository.

## Building

You will need to make sure that you have Go installed on your system and the containerd repository is cloned
in your `$GOPATH`.  You will also need to make sure that you have all the dependencies cloned as well.
Currently, contributing to containerd is not for the first time devs as many dependencies are not vendored and 
work is being completed at a high rate.  

After that just run `make` and the binaries for the daemon and client will be localed in the `bin/` directory.

## Downloads

The easy way to test and use containerd is to view the [releases page](https://github.com/docker/containerd/releases) for binary downloads.
We encourage everyone to use containerd this way until it is out of alpha status.

## Performance

Starting 1000 containers concurrently runs at 126-140 containers per second.

Overall start times:

```
[containerd] 2015/12/04 15:00:54   count:        1000
[containerd] 2015/12/04 14:59:54   min:          23ms
[containerd] 2015/12/04 14:59:54   max:         355ms
[containerd] 2015/12/04 14:59:54   mean:         78ms
[containerd] 2015/12/04 14:59:54   stddev:       34ms
[containerd] 2015/12/04 14:59:54   median:       73ms
[containerd] 2015/12/04 14:59:54   75%:          91ms
[containerd] 2015/12/04 14:59:54   95%:         123ms
[containerd] 2015/12/04 14:59:54   99%:         287ms
[containerd] 2015/12/04 14:59:54   99.9%:       355ms
```

## Telemetry 

Currently containerd only outputs metrics to stdout but will support dumping to various backends in the future.

```
[containerd] 2015/12/16 11:48:28 timer container-start-time
[containerd] 2015/12/16 11:48:28   count:              22
[containerd] 2015/12/16 11:48:28   min:          25425883
[containerd] 2015/12/16 11:48:28   max:         113077691
[containerd] 2015/12/16 11:48:28   mean:         68386923.27
[containerd] 2015/12/16 11:48:28   stddev:       20928453.26
[containerd] 2015/12/16 11:48:28   median:       65489003.50
[containerd] 2015/12/16 11:48:28   75%:          82393210.50
[containerd] 2015/12/16 11:48:28   95%:         112267814.75
[containerd] 2015/12/16 11:48:28   99%:         113077691.00
[containerd] 2015/12/16 11:48:28   99.9%:       113077691.00
[containerd] 2015/12/16 11:48:28   1-min rate:          0.00
[containerd] 2015/12/16 11:48:28   5-min rate:          0.01
[containerd] 2015/12/16 11:48:28   15-min rate:         0.01
[containerd] 2015/12/16 11:48:28   mean rate:           0.03
[containerd] 2015/12/16 11:48:28 counter containers
[containerd] 2015/12/16 11:48:28   count:               1
[containerd] 2015/12/16 11:48:28 counter events
[containerd] 2015/12/16 11:48:28   count:              87
[containerd] 2015/12/16 11:48:28 counter events-subscribers
[containerd] 2015/12/16 11:48:28   count:               2
[containerd] 2015/12/16 11:48:28 gauge goroutines
[containerd] 2015/12/16 11:48:28   value:              38
[containerd] 2015/12/16 11:48:28 gauge fds
[containerd] 2015/12/16 11:48:28   value:              18
```

## Daemon options


```
$ containerd -h

NAME:
   containerd - High performance container daemon

USAGE:
   containerd [global options] command [command options] [arguments...]

VERSION:
   0.0.4

AUTHOR(S): 
   @crosbymichael <crosbymichael@gmail.com> 

COMMANDS:
   help, h  Shows a list of commands or help for one command
   
GLOBAL OPTIONS:
   --id "deathstar"                 unique containerd id to identify the instance
   --debug                      enable debug output in the logs
   --state-dir "/run/containerd"            runtime state directory
   -c, --concurrency "10"               set the concurrency level for tasks
   --metrics-interval "1m0s"                interval for flushing metrics to the store
   --listen, -l "/run/containerd/containerd.sock"   Address on which GRPC API will listen
   --oom-notify                     enable oom notifications for containers
   --help, -h                       show help
   --version, -v                    print the version
   
```

# Roadmap 

The current roadmap and milestones for alpha and beta completion are in the github issues on this repository.  Please refer to these issues for what is being worked on and completed for the various stages of development.

# API

## GRPC API

The API for containerd is with GRPC over a unix socket located at the default location of `/run/containerd/containerd.sock`.  

At this time please refer to the [proto at](https://github.com/docker/containerd/blob/master/api/grpc/types/api.proto) for the API methods and types.  
There is a Go implementation and types checked into this repository but alternate language implementations can be created using the grpc and protoc toolchain.


## containerd CLI

There is a default cli named `ctr` based on the GRPC api.
This cli will allow you to create and manage containers run with containerd.

```
NAME:
   ctr - High performance container daemon controller

USAGE:
   ctr [global options] command [command options] [arguments...]

VERSION:
   0.0.4

AUTHOR(S): 
   @crosbymichael <crosbymichael@gmail.com> 

COMMANDS:
   checkpoints  list all checkpoints
   containers   interact with running containers
   events   receive events from the containerd daemon
   help, h  Shows a list of commands or help for one command
   
GLOBAL OPTIONS:
   --debug                  enable debug output in the logs
   --address "/run/containerd/containerd.sock"  address of GRPC API
   --help, -h                   show help
   --version, -v                print the version
   
```

### Listing containers

```bash
$ sudo ctr containers
ID                  PATH                STATUS              PID1
1                   /containers/redis   running             14063
19                  /containers/redis   running             14100
14                  /containers/redis   running             14117
4                   /containers/redis   running             14030
16                  /containers/redis   running             14061
3                   /containers/redis   running             14024
12                  /containers/redis   running             14097
10                  /containers/redis   running             14131
18                  /containers/redis   running             13977
13                  /containers/redis   running             13979
15                  /containers/redis   running             13998
5                   /containers/redis   running             14021
9                   /containers/redis   running             14075
6                   /containers/redis   running             14107
2                   /containers/redis   running             14135
11                  /containers/redis   running             13978
17                  /containers/redis   running             13989
8                   /containers/redis   running             14053
7                   /containers/redis   running             14022
0                   /containers/redis   running             14006
```

### Starting a container

```
$ ctr containers start -h
NAME:
   start - start a container

USAGE:
   command start [command options] [arguments...]

OPTIONS:
   --checkpoint, -c     checkpoint to start the container from
   --attach, -a         connect to the stdio of the container

```

```bash
$ sudo ctr containers start redis /containers/redis
```

### Kill a container's process

```
$ ctr containers kill -h 
NAME:
   kill - send a signal to a container or it's processes

USAGE:
   command kill [command options] [arguments...]

OPTIONS:
   --pid, -p "0"        pid of the process to signal within the container
   --signal, -s "15"    signal to send to the container
```

### Exec another process into a container

```
$ ctr containers exec -h 
NAME:
   exec - exec another process in an existing container

USAGE:
   command exec [command options] [arguments...]

OPTIONS:
   --id                                         container id to add the process to
   --attach, -a                                 connect to the stdio of the container
   --cwd                                        current working directory for the process
   --tty, -t                                    create a terminal for the process
   --env, -e [--env option --env option]        environment variables for the process
   --uid, -u "0"                                user id of the user for the process
   --gid, -g "0"                                group id of the user for the process
   
```

### Stats for a container

```
$ ctr containers stats -h 
NAME:
   stats - get stats for running container

USAGE:
  command stats [arguments...]
```

### List checkpoints

```
$ sudo ctr checkpoints redis
NAME                TCP                 UNIX SOCKETS        SHELL
test                false               false               false
test2               false               false               false
```

### Create a new checkpoint

```
$ ctr checkpoints create -h
NAME:
   create - create a new checkpoint for the container

USAGE:
   command create [command options] [arguments...]

OPTIONS:
   --tcp                persist open tcp connections
   --unix-sockets       perist unix sockets
   --exit               exit the container after the checkpoint completes successfully
   --shell              checkpoint shell jobs
   
```

### Get events

```
$ sudo ctr events
TYPE                ID                  PID                 STATUS
exit                redis               24761               0
```


### Sign your work

The sign-off is a simple line at the end of the explanation for the patch. Your
signature certifies that you wrote the patch or otherwise have the right to pass
it on as an open-source patch. The rules are pretty simple: if you can certify
the below (from [developercertificate.org](http://developercertificate.org/)):

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
660 York Street, Suite 102,
San Francisco, CA 94110 USA

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.

Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

Then you just add a line to every git commit message:

    Signed-off-by: Joe Smith <joe.smith@email.com>

Use your real name (sorry, no pseudonyms or anonymous contributions.)

If you set your `user.name` and `user.email` git configs, you can sign your
commit automatically with `git commit -s`.

## Copyright and license

Copyright © 2015 Docker, Inc. All rights reserved, except as follows. Code
is released under the Apache 2.0 license. The README.md file, and files in the
"docs" folder are licensed under the Creative Commons Attribution 4.0
International License under the terms and conditions set forth in the file
"LICENSE.docs". You may obtain a duplicate copy of the same license, titled
CC-BY-SA-4.0, at http://creativecommons.org/licenses/by/4.0/.
