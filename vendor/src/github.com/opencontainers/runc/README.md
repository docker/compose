[![Build Status](https://jenkins.dockerproject.org/buildStatus/icon?job=runc Master)](https://jenkins.dockerproject.org/job/runc Master)

## runc

`runc` is a CLI tool for spawning and running containers according to the OCF specification.

## State of the project

Currently `runc` is an implementation of the OCI specification.  We are currently sprinting
to have a v1 of the spec out. So the `runc` config format will be constantly changing until
the spec is finalized. However, we encourage you to try out the tool and give feedback.

### OCF

How does `runc` integrate with the Open Container Initiative Specification?
`runc` depends on the types specified in the
[specs](https://github.com/opencontainers/specs) repository. Whenever the
specification is updated and ready to be versioned `runc` will update its dependency
on the specs repository and support the update spec.

### Building:

At the time of writing, runc only builds on the Linux platform.

```bash
# create a 'github.com/opencontainers' in your GOPATH/src
cd github.com/opencontainers
git clone https://github.com/opencontainers/runc
cd runc
make
sudo make install
```

In order to enable seccomp support you will need to install libseccomp on your platform.
If you do not with to build `runc` with seccomp support you can add `BUILDTAGS=""` when running make.

#### Build Tags

`runc` supports optional build tags for compiling in support for various features.


| Build Tag | Feature                            | Dependency  |
|-----------|------------------------------------|-------------|
| seccomp   | Syscall filtering                  | libseccomp  |
| selinux   | selinux process and mount labeling | <none>      |
| apparmor  | apparmor profile support           | libapparmor |

### Testing:

You can run tests for runC by using command:

```bash
# make test
```

Note that test cases are run in Docker container, so you need to install
`docker` first. And test requires mounting cgroups inside container, it's
done by docker now, so you need a docker version newer than 1.8.0-rc2.

You can also run specific test cases by:

```bash
# make test TESTFLAGS="-run=SomeTestFunction"
```

### Using:

To run a container with the id "test", execute `runc start` with the containers id as arg one 
in the bundle's root directory:

```bash
runc start test
/ $ ps
PID   USER     COMMAND
1     daemon   sh
5     daemon   sh
/ $
```

### OCI Container JSON Format:

OCI container JSON format is based on OCI [specs](https://github.com/opencontainers/specs).
You can generate JSON files by using `runc spec`.
It assumes that the file-system is found in a directory called
`rootfs` and there is a user with uid and gid of `0` defined within that file-system.

### Examples:

#### Using a Docker image (requires version 1.3 or later)

To test using Docker's `busybox` image follow these steps:
* Install `docker` and download the `busybox` image: `docker pull busybox`
* Create a container from that image and export its contents to a tar file:
`docker export $(docker create busybox) > busybox.tar`
* Untar the contents to create your filesystem directory:
```
mkdir rootfs
tar -C rootfs -xf busybox.tar
```
* Create `config.json` by using `runc spec`.
* Execute `runc start` and you should be placed into a shell where you can run `ps`:
```
$ runc start test
/ # ps
PID   USER     COMMAND
    1 root     sh
    9 root     ps
```

#### Using runc with systemd

To use runc with systemd, you can create a unit file
`/usr/lib/systemd/system/minecraft.service` as below (edit your
own Description or WorkingDirectory or service name as you need).

```service
[Unit]
Description=Minecraft Build Server
Documentation=http://minecraft.net
After=network.target

[Service]
CPUQuota=200%
MemoryLimit=1536M
ExecStart=/usr/local/bin/runc start minecraft
Restart=on-failure
WorkingDirectory=/containers/minecraftbuild

[Install]
WantedBy=multi-user.target
```

Make sure you have the bundle's root directory and JSON configs in
your WorkingDirectory, then use systemd commands to start the service:

```bash
systemctl daemon-reload
systemctl start minecraft.service
```

Note that if you use JSON configs by `runc spec`, you need to modify
`config.json` and change `process.terminal` to false so runc won't
create tty, because we can't set terminal from the stdin when using
systemd service.
