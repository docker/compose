# Client CLI

There is a default cli named `ctr` based on the GRPC api.
This cli will allow you to create and manage containers run with containerd.

```
$ ctr -h
NAME:
   ctr - High performance container daemon cli

USAGE:
   ctr [global options] command [command options] [arguments...]

VERSION:
   0.1.0 commit: 54c213e8a719d734001beb2cb8f130c84cc3bd20

COMMANDS:
   checkpoints  list all checkpoints
   containers   interact with running containers
   events       receive events from the containerd daemon
   state        get a raw dump of the containerd state
   help, h      Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug                                      enable debug output in the logs
   --address "/run/containerd/containerd.sock"  address of GRPC API
   --help, -h                                   show help
   --version, -v                                print the version
```

## Starting a container

```
$ ctr containers start -h
NAME:
   ctr containers start - start a container

USAGE:
   ctr containers start [command options] [arguments...]

OPTIONS:
   --checkpoint, -c                             checkpoint to start the container from
   --attach, -a                                 connect to the stdio of the container
   --label, -l [--label option --label option]  set labels for the container
```

```bash
$ sudo ctr containers start redis /containers/redis
```

`/containers/redis` is the path to an OCI bundle. [See the bundle docs for more information.](bundle.md)

## Listing containers

```bash
$ sudo ctr containers
ID                  PATH                STATUS              PROCESSES
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

## Kill a container's process

```
$ ctr containers kill -h
NAME:
   ctr containers kill - send a signal to a container or its processes

USAGE:
   ctr containers kill [command options] [arguments...]

OPTIONS:
   --pid, -p "init"     pid of the process to signal within the container
   --signal, -s "15"    signal to send to the container
```

## Exec another process into a container

```
$ ctr containers exec -h
NAME:
   ctr containers exec - exec another process in an existing container

USAGE:
   ctr containers exec [command options] [arguments...]

OPTIONS:
   --id                                         container id to add the process to
   --pid                                        process id for the new process
   --attach, -a                                 connect to the stdio of the container
   --cwd                                        current working directory for the process
   --tty, -t                                    create a terminal for the process
   --env, -e [--env option --env option]        environment variables for the process
   --uid, -u "0"                                user id of the user for the process
   --gid, -g "0"                                group id of the user for the process
```

## Stats for a container

```
$ ctr containers stats -h
NAME:
   ctr containers stats - get stats for running container

USAGE:
   ctr containers stats [arguments...]
```

## List checkpoints

```
$ sudo ctr checkpoints redis
NAME                TCP                 UNIX SOCKETS        SHELL
test                false               false               false
test2               false               false               false
```

## Create a new checkpoint

```
$ ctr checkpoints create -h
NAME:
   ctr checkpoints create - create a new checkpoint for the container

USAGE:
   ctr checkpoints create [command options] [arguments...]

OPTIONS:
   --tcp                persist open tcp connections
   --unix-sockets       perist unix sockets
   --exit               exit the container after the checkpoint completes successfully
   --shell              checkpoint shell jobs
```

## Get events

```
$ sudo ctr events
TYPE                ID                  PID                 STATUS
exit                redis               24761               0
```
