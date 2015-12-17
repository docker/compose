# OCI Bundle

Since containerd consumes the OCI bundle format containers and configuration will have to be created
on the machine that containerd is running on.  The easiest way to do this is to download an image 
with docker and export it.


## Setup

First thing we need to do to create a bundle is setup the initial directory structure.
Create a directory with a unique name.  In this example we will create a redis container.
We will create this container in a `/containers` directory.


```bash
mkdir redis
```

Inside the `redis` directory create another directory named `rootfs`

```bash
mkdir redis/rootfs
```

## Root Filesystem

Now we need to populate the `rootfs` directory with the filesystem of a redis container.  To do this we
need to pull the redis image with docker and export its contents to the `rootfs` directory.

```bash
docker pull redis

# create the container with a temp name so that we can export it
docker create --name tempredis redis

# export it into the rootfs directory
docker export tempredis | tar -C redis/rootfs -xf -

# remove the container now that we have exported
docker rm tempredis
```

Now that we have the root filesystem populated we need to create the configs for the container.

## Configs

An easy way to get temp configs for the container bundle is to use the `runc` 
cli tool from the [runc](https://github.com/opencontainers/runc) repository.


You need to `cd` into the `redis` directory and run the `runc spec` command.  After doing this you
should have two files created, `configs.json` and `runtime.json`.  The directory structure should 
look like this:

```
/containers/redis
├── config.json
├── rootfs/
└── runtime.json
```

## Edits

We need to edit the config to add `redis-server` as the application to launch inside the container along with 
a few other settings.  The resulting `config.json` should look like this:

```json
{
    "version": "0.2.0",
    "platform": {
        "os": "linux",
        "arch": "amd64"
    },
    "process": {
        "terminal": false,
        "user": {
            "uid": 1000,
            "gid": 1000
        },
        "args": [
            "redis-server", "--bind", "0.0.0.0"
        ],
        "env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "TERM=xterm"
        ],
        "cwd": ""
    },
    "root": {
        "path": "rootfs",
        "readonly": false
    },
    "hostname": "shell",
    "mounts": [
        {"name": "proc", "path": "/proc"},
        {"name": "dev", "path": "/dev"},
        {"name": "devpts", "path": "/dev/pts"}, 
        {"name": "shm", "path": "/dev/shm"},
        {"name": "mqueue", "path": "/dev/mqueue"},
        {"name": "sysfs", "path": "/sys"}
    ],
    "linux": {
        "capabilities": [
            "CAP_AUDIT_WRITE",
            "CAP_KILL",
            "CAP_NET_BIND_SERVICE"
        ]
    }
}
```

You will also want to edit the `runtime.json` file to remove the network namespace so that
you can connect to the redis server on your system.  The final result for the `runtime.json`
file should look like this:

```json
{
    "mounts": {
        "dev": {
            "type": "tmpfs",
            "source": "tmpfs",
            "options": [
                "nosuid",
                "strictatime",
                "mode=755",
                "size=65536k"
            ]
        },
        "devpts": {
            "type": "devpts",
            "source": "devpts",
            "options": [
                "nosuid",
                "noexec",
                "newinstance",
                "ptmxmode=0666",
                "mode=0620",
                "gid=5"
            ]
        },
        "mqueue": {
            "type": "mqueue",
            "source": "mqueue",
            "options": [
                "nosuid",
                "noexec",
                "nodev"
            ]
        },
        "proc": {
            "type": "proc",
            "source": "proc",
            "options": null
        },
        "shm": {
            "type": "tmpfs",
            "source": "shm",
            "options": [
                "nosuid",
                "noexec",
                "nodev",
                "mode=1777",
                "size=65536k"
            ]
        },
        "sysfs": {
            "type": "sysfs",
            "source": "sysfs",
            "options": [
                "nosuid",
                "noexec",
                "nodev"
            ]
        }
    },
    "linux": {
        "rlimits": [
            {
                "type": "RLIMIT_NOFILE",
                "hard": 1024,
                "soft": 1024
            }
        ],
        "resources": {
            "disableOOMKiller": false,
            "memory": {
                "limit": 0,
                "reservation": 0,
                "swap": 0,
                "kernel": 0
            },
            "cpu": {
                "shares": 0,
                "quota": 0,
                "period": 0,
                "realtimeRuntime": 0,
                "realtimePeriod": 0,
                "cpus": "",
                "mems": ""
            },
            "pids": {
                "limit": 0
            },
            "blockIO": {
                "blkioWeight": 0,
                "blkioLeafWeight": 0,
                "blkioWeightDevice": null,
                "blkioThrottleReadBpsDevice": null,
                "blkioThrottleWriteBpsDevice": null,
                "blkioThrottleReadIOPSDevice": null,
                "blkioThrottleWriteIOPSDevice": null
            },
            "hugepageLimits": null,
            "network": {
                "classId": "",
                "priorities": null
            }
        },
        "namespaces": [
            {"type": "pid", "path": ""},
            {"type": "ipc", "path": ""},
            {"type": "uts", "path": ""},
            {"type": "mount", "path": ""}
        ],
        "devices": [
            {
                "path": "/dev/null",
                "type": 99,
                "major": 1,
                "minor": 3,
                "permissions": "rwm",
                "fileMode": 438,
                "uid": 0,
                "gid": 0
            },
            {
                "path": "/dev/random",
                "type": 99,
                "major": 1,
                "minor": 8,
                "permissions": "rwm",
                "fileMode": 438,
                "uid": 0,
                "gid": 0
            },
            {
                "path": "/dev/full",
                "type": 99,
                "major": 1,
                "minor": 7,
                "permissions": "rwm",
                "fileMode": 438,
                "uid": 0,
                "gid": 0
            },
            {
                "path": "/dev/tty",
                "type": 99,
                "major": 5,
                "minor": 0,
                "permissions": "rwm",
                "fileMode": 438,
                "uid": 0,
                "gid": 0
            },
            {
                "path": "/dev/zero",
                "type": 99,
                "major": 1,
                "minor": 5,
                "permissions": "rwm",
                "fileMode": 438,
                "uid": 0,
                "gid": 0
            },
            {
                "path": "/dev/urandom",
                "type": 99,
                "major": 1,
                "minor": 9,
                "permissions": "rwm",
                "fileMode": 438,
                "uid": 0,
                "gid": 0
            }
        ]
    }
}
```

This is what you need to do to make a OCI compliant bundle for containerd to start.
