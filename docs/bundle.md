# Creating OCI bundles

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
should have a file `config.json` created.  The directory structure should look like this:

```
/containers/redis
├── config.json
└── rootfs/
```

## Edits

We need to edit the config to add `redis-server` as the application to launch inside the container,
and remove the network namespace so that you can connect to the redis server on your system.
The resulting `config.json` should look like this:

```json
{
	"ociVersion": "0.4.0",
	"platform": {
		"os": "linux",
		"arch": "amd64"
	},
	"process": {
		"terminal": true,
		"user": {},
		"args": [
			"redis-server", "--bind", "0.0.0.0"
		],
		"env": [
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"TERM=xterm"
		],
		"cwd": "/",
		"capabilities": [
			"CAP_AUDIT_WRITE",
			"CAP_KILL",
			"CAP_NET_BIND_SERVICE"
		],
		"rlimits": [
			{
				"type": "RLIMIT_NOFILE",
				"hard": 1024,
				"soft": 1024
			}
		],
		"noNewPrivileges": true
	},
	"root": {
		"path": "rootfs",
		"readonly": true
	},
	"hostname": "runc",
	"mounts": [
		{
			"destination": "/proc",
			"type": "proc",
			"source": "proc"
		},
		{
			"destination": "/dev",
			"type": "tmpfs",
			"source": "tmpfs",
			"options": [
				"nosuid",
				"strictatime",
				"mode=755",
				"size=65536k"
			]
		},
		{
			"destination": "/dev/pts",
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
		{
			"destination": "/dev/shm",
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
		{
			"destination": "/dev/mqueue",
			"type": "mqueue",
			"source": "mqueue",
			"options": [
				"nosuid",
				"noexec",
				"nodev"
			]
		},
		{
			"destination": "/sys",
			"type": "sysfs",
			"source": "sysfs",
			"options": [
				"nosuid",
				"noexec",
				"nodev",
				"ro"
			]
		},
		{
			"destination": "/sys/fs/cgroup",
			"type": "cgroup",
			"source": "cgroup",
			"options": [
				"nosuid",
				"noexec",
				"nodev",
				"relatime",
				"ro"
			]
		}
	],
	"hooks": {},
	"linux": {
		"resources": {
			"devices": [
				{
					"allow": false,
					"access": "rwm"
				}
			]
		},
		"namespaces": [
			{
				"type": "pid"
			},
			{
				"type": "ipc"
			},
			{
				"type": "uts"
			},
			{
				"type": "mount"
			}
		],
		"devices": null
	}
}
```

This is what you need to do to make a OCI compliant bundle for containerd to start.
