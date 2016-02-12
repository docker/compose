# Linux-specific Container Configuration

The Linux container specification uses various kernel features like namespaces, cgroups, capabilities, LSM, and file system jails to fulfill the spec.
Additional information is needed for Linux over the [default spec configuration](config.md) in order to configure these various kernel features.

## Capabilities

Capabilities is an array that specifies Linux capabilities that can be provided to the process inside the container.
Valid values are the strings for capabilities defined in [the man page](http://man7.org/linux/man-pages/man7/capabilities.7.html)

```json
   "capabilities": [
        "CAP_AUDIT_WRITE",
        "CAP_KILL",
        "CAP_NET_BIND_SERVICE"
    ]
```

## Default File Systems

The Linux ABI includes both syscalls and several special file paths.
Applications expecting a Linux environment will very likely expect these files paths to be setup correctly.

The following filesystems MUST be made available in each application's filesystem

|   Path   |  Type  |
| -------- | ------ |
| /proc    | [procfs](https://www.kernel.org/doc/Documentation/filesystems/proc.txt)   |
| /sys     | [sysfs](https://www.kernel.org/doc/Documentation/filesystems/sysfs.txt)   |
| /dev/pts | [devpts](https://www.kernel.org/doc/Documentation/filesystems/devpts.txt) |
| /dev/shm | [tmpfs](https://www.kernel.org/doc/Documentation/filesystems/tmpfs.txt)   |

## Namespaces

A namespace wraps a global system resource in an abstraction that makes it appear to the processes within the namespace that they have their own isolated instance of the global resource.
Changes to the global resource are visible to other processes that are members of the namespace, but are invisible to other processes.
For more information, see [the man page](http://man7.org/linux/man-pages/man7/namespaces.7.html).

Namespaces are specified as an array of entries inside the `namespaces` root field.
The following parameters can be specified to setup namespaces:

* **`type`** *(string, required)* - namespace type. The following namespaces types are supported:
    * **`pid`** processes inside the container will only be able to see other processes inside the same container
    * **`network`** the container will have its own network stack
    * **`mount`** the container will have an isolated mount table
    * **`ipc`** processes inside the container will only be able to communicate to other processes inside the same container via system level IPC
    * **`uts`** the container will be able to have its own hostname and domain name
    * **`user`** the container will be able to remap user and group IDs from the host to local users and groups within the container

* **`path`** *(string, optional)* - path to namespace file

If a path is specified, that particular file is used to join that type of namespace.
Also, when a path is specified, a runtime MUST assume that the setup for that particular namespace has already been done and error out if the config specifies anything else related to that namespace.

###### Example

```json
    "namespaces": [
        {
            "type": "pid",
            "path": "/proc/1234/ns/pid"
        },
        {
            "type": "network",
            "path": "/var/run/netns/neta"
        },
        {
            "type": "mount"
        },
        {
            "type": "ipc"
        },
        {
            "type": "uts"
        },
        {
            "type": "user"
        }
    ]
```

## User namespace mappings

###### Example

```json
    "uidMappings": [
        {
            "hostID": 1000,
            "containerID": 0,
            "size": 10
        }
    ],
    "gidMappings": [
        {
            "hostID": 1000,
            "containerID": 0,
            "size": 10
        }
    ]
```

uid/gid mappings describe the user namespace mappings from the host to the container.
The mappings represent how the bundle `rootfs` expects the user namespace to be setup and the runtime SHOULD NOT modify the permissions on the rootfs to realize the mapping.
*hostID* is the starting uid/gid on the host to be mapped to *containerID* which is the starting uid/gid in the container and *size* refers to the number of ids to be mapped.
There is a limit of 5 mappings which is the Linux kernel hard limit.

## Devices

`devices` is an array specifying the list of devices that MUST be available in the container.
The runtime may supply them however it likes (with [mknod][mknod.2], by bind mounting from the runtime mount namespace, etc.).

The following parameters can be specified:

* **`type`** *(char, required)* - type of device: `c`, `b`, `u` or `p`.
  More info in [mknod(1)][mknod.1].
* **`path`** *(string, required)* - full path to device inside container.
* **`major, minor`** *(int64, required unless **`type`** is `p`)* - [major, minor numbers][devices] for the device.
* **`fileMode`** *(uint32, optional)* - file mode for the device.
  You can also control access to devices [with cgroups](#device-whitelist).
* **`uid`** *(uint32, optional)* - id of device owner.
* **`gid`** *(uint32, optional)* - id of device group.

###### Example

```json
   "devices": [
        {
            "path": "/dev/fuse",
            "type": "c",
            "major": 10,
            "minor": 229,
            "fileMode": 0666,
            "uid": 0,
            "gid": 0
        },
        {
            "path": "/dev/sda",
            "type": "b",
            "major": 8,
            "minor": 0,
            "fileMode": 0660,
            "uid": 0,
            "gid": 0
        }
    ]
```

###### Default Devices

In addition to any devices configured with this setting, the runtime MUST also supply:

* [`/dev/null`][null.4]
* [`/dev/zero`][zero.4]
* [`/dev/full`][full.4]
* [`/dev/random`][random.4]
* [`/dev/urandom`][random.4]
* [`/dev/tty`][tty.4]
* [`/dev/console`][console.4]
* [`/dev/ptmx`][pts.4].
  A [bind-mount or symlink of the container's `/dev/pts/ptmx`][devpts].

## Control groups

Also known as cgroups, they are used to restrict resource usage for a container and handle device access.
cgroups provide controls to restrict cpu, memory, IO, pids and network for the container.
For more information, see the [kernel cgroups documentation][cgroup-v1].

The path to the cgroups can be specified in the Spec via `cgroupsPath`.
`cgroupsPath` is expected to be relative to the cgroups mount point.
If `cgroupsPath` is not specified, implementations can define the default cgroup path.
Implementations of the Spec can choose to name cgroups in any manner.
The Spec does not include naming schema for cgroups.
The Spec does not support [split hierarchy][cgroup-v2].
The cgroups will be created if they don't exist.

###### Example

```json
   "cgroupsPath": "/myRuntime/myContainer"
```

`cgroupsPath` can be used to either control the cgroups hierarchy for containers or to run a new process in an existing container.

You can configure a container's cgroups via the `resources` field of the Linux configuration.
Do not specify `resources` unless limits have to be updated.
For example, to run a new process in an existing container without updating limits, `resources` need not be specified.

#### Device whitelist

`devices` is an array of entries to control the [device whitelist][cgroup-v1-devices].
The runtime MUST apply entries in the listed order.

The following parameters can be specified:

* **`allow`** *(boolean, required)* - whether the entry is allowed or denied.
* **`type`** *(char, optional)* - type of device: `a` (all), `c` (char), or `b` (block).
  `null` or unset values mean "all", mapping to `a`.
* **`major, minor`** *(int64, optional)* - [major, minor numbers][devices] for the device.
  `null` or unset values mean "all", mapping to [`*` in the filesystem API][cgroup-v1-devices].
* **`access`** *(string, optional)* - cgroup permissions for device.
  A composition of `r` (read), `w` (write), and `m` (mknod).

###### Example

```json
   "devices": [
        {
            "allow": false,
            "access": "rwm"
        },
        {
            "allow": true,
            "type": "c",
            "major": 10,
            "minor": 229,
            "access": "rw"
        },
        {
            "allow": true,
            "type": "b",
            "major": 8,
            "minor": 0,
            "access": "r"
        }
    ]
```

#### Disable out-of-memory killer

`disableOOMKiller` contains a boolean (`true` or `false`) that enables or disables the Out of Memory killer for a cgroup.
If enabled (`false`), tasks that attempt to consume more memory than they are allowed are immediately killed by the OOM killer.
The OOM killer is enabled by default in every cgroup using the `memory` subsystem.
To disable it, specify a value of `true`.
For more information, see [the memory cgroup man page][cgroup-v1-memory].

* **`disableOOMKiller`** *(bool, optional)* - enables or disables the OOM killer

###### Example

```json
    "disableOOMKiller": false
```

#### Set oom_score_adj

`oomScoreAdj` sets heuristic regarding how the process is evaluated by the kernel during memory pressure.
For more information, see [the proc filesystem documentation section 3.1](https://www.kernel.org/doc/Documentation/filesystems/proc.txt).
This is a kernel/system level setting, where as `disableOOMKiller` is scoped for a memory cgroup.
For more information on how these two settings work together, see [the memory cgroup documentation section 10. OOM Contol][cgroup-v1-memory].

* **`oomScoreAdj`** *(int, optional)* - adjust the oom-killer score

###### Example

```json
    "oomScoreAdj": 0
```

#### Memory

`memory` represents the cgroup subsystem `memory` and it's used to set limits on the container's memory usage.
For more information, see [the memory cgroup man page][cgroup-v1-memory].

The following parameters can be specified to setup the controller:

* **`limit`** *(uint64, optional)* - sets limit of memory usage

* **`reservation`** *(uint64, optional)* - sets soft limit of memory usage

* **`swap`** *(uint64, optional)* - sets limit of memory+Swap usage

* **`kernel`** *(uint64, optional)* - sets hard limit for kernel memory

* **`kernelTCP`** *(uint64, optional)* - sets hard limit for kernel memory in tcp using

* **`swappiness`** *(uint64, optional)* - sets swappiness parameter of vmscan (See sysctl's vm.swappiness)

###### Example

```json
    "memory": {
        "limit": 0,
        "reservation": 0,
        "swap": 0,
        "kernel": 0,
        "kernelTCP": 0,
        "swappiness": 0
    }
```

#### CPU

`cpu` represents the cgroup subsystems `cpu` and `cpusets`.
For more information, see [the cpusets cgroup man page][cgroup-v1-cpusets].

The following parameters can be specified to setup the controller:

* **`shares`** *(uint64, optional)* - specifies a relative share of CPU time available to the tasks in a cgroup

* **`quota`** *(uint64, optional)* - specifies the total amount of time in microseconds for which all tasks in a cgroup can run during one period (as defined by **`period`** below)

* **`period`** *(uint64, optional)* - specifies a period of time in microseconds for how regularly a cgroup's access to CPU resources should be reallocated (CFS scheduler only)

* **`realtimeRuntime`** *(uint64, optional)* - specifies a period of time in microseconds for the longest continuous period in which the tasks in a cgroup have access to CPU resources

* **`realtimePeriod`** *(uint64, optional)* - same as **`period`** but applies to realtime scheduler only

* **`cpus`** *(string, optional)* - list of CPUs the container will run in

* **`mems`** *(string, optional)* - list of Memory Nodes the container will run in

###### Example

```json
    "cpu": {
        "shares": 0,
        "quota": 0,
        "period": 0,
        "realtimeRuntime": 0,
        "realtimePeriod": 0,
        "cpus": "",
        "mems": ""
    }
```

#### Block IO Controller

`blockIO` represents the cgroup subsystem `blkio` which implements the block io controller.
For more information, see [the kernel cgroups documentation about blkio][cgroup-v1-blkio].

The following parameters can be specified to setup the controller:

* **`blkioWeight`** *(uint16, optional)* - specifies per-cgroup weight. This is default weight of the group on all devices until and unless overridden by per-device rules. The range is from 10 to 1000.

* **`blkioLeafWeight`** *(uint16, optional)* - equivalents of `blkioWeight` for the purpose of deciding how much weight tasks in the given cgroup has while competing with the cgroup's child cgroups. The range is from 10 to 1000.

* **`blkioWeightDevice`** *(array, optional)* - specifies the list of devices which will be bandwidth rate limited. The following parameters can be specified per-device:
    * **`major, minor`** *(int64, required)* - major, minor numbers for device. More info in `man mknod`.
    * **`weight`** *(uint16, optional)* - bandwidth rate for the device, range is from 10 to 1000
    * **`leafWeight`** *(uint16, optional)* - bandwidth rate for the device while competing with the cgroup's child cgroups, range is from 10 to 1000, CFQ scheduler only

    You must specify at least one of `weight` or `leafWeight` in a given entry, and can specify both.

* **`blkioThrottleReadBpsDevice`**, **`blkioThrottleWriteBpsDevice`**, **`blkioThrottleReadIOPSDevice`**, **`blkioThrottleWriteIOPSDevice`** *(array, optional)* - specify the list of devices which will be IO rate limited. The following parameters can be specified per-device:
    * **`major, minor`** *(int64, required)* - major, minor numbers for device. More info in `man mknod`.
    * **`rate`** *(uint64, required)* - IO rate limit for the device

###### Example

```json
    "blockIO": {
        "blkioWeight": 0,
        "blkioLeafWeight": 0,
        "blkioWeightDevice": [
            {
                "major": 8,
                "minor": 0,
                "weight": 500,
                "leafWeight": 300
            },
            {
                "major": 8,
                "minor": 16,
                "weight": 500
            }
        ],
        "blkioThrottleReadBpsDevice": [
            {
                "major": 8,
                "minor": 0,
                "rate": 600
            }
        ],
        "blkioThrottleWriteIOPSDevice": [
            {
                "major": 8,
                "minor": 16,
                "rate": 300
            }
        ]
    }
```

#### Huge page limits

`hugepageLimits` represents the `hugetlb` controller which allows to limit the
HugeTLB usage per control group and enforces the controller limit during page fault.
For more information, see the [kernel cgroups documentation about HugeTLB][cgroup-v1-hugetlb].

`hugepageLimits` is an array of entries, each having the following structure:

* **`pageSize`** *(string, required)* - hugepage size

* **`limit`** *(uint64, required)* - limit in bytes of *hugepagesize* HugeTLB usage

###### Example

```json
   "hugepageLimits": [
        {
            "pageSize": "2MB",
            "limit": 9223372036854771712
        }
   ]
```

#### Network

`network` represents the cgroup subsystems `net_cls` and `net_prio`.
For more information, see [the net\_cls cgroup man page][cgroup-v1-net-cls] and [the net\_prio cgroup man page][cgroup-v1-net-prio].

The following parameters can be specified to setup these cgroup controllers:

* **`classID`** *(uint32, optional)* - is the network class identifier the cgroup's network packets will be tagged with

* **`priorities`** *(array, optional)* - specifies a list of objects of the priorities assigned to traffic originating from
processes in the group and egressing the system on various interfaces. The following parameters can be specified per-priority:
    * **`name`** *(string, required)* - interface name
    * **`priority`** *(uint32, required)* - priority applied to the interface

###### Example

```json
   "network": {
        "classID": 1048577,
        "priorities": [
            {
                "name": "eth0",
                "priority": 500
            },
            {
                "name": "eth1",
                "priority": 1000
            }
        ]
   }
```

#### PIDs

`pids` represents the cgroup subsystem `pids`.
For more information, see [the pids cgroup man page][cgroup-v1-pids].

The following paramters can be specified to setup the controller:

* **`limit`** *(int64, required)* - specifies the maximum number of tasks in the cgroup

###### Example

```json
   "pids": {
        "limit": 32771
   }
```

## Sysctl

sysctl allows kernel parameters to be modified at runtime for the container.
For more information, see [the man page](http://man7.org/linux/man-pages/man8/sysctl.8.html)

###### Example

```json
   "sysctl": {
        "net.ipv4.ip_forward": "1",
        "net.core.somaxconn": "256"
   }
```

## Rlimits

rlimits allow setting resource limits.
`type` is a string with a value from those defined in [the man page](http://man7.org/linux/man-pages/man2/setrlimit.2.html).
The kernel enforces the `soft` limit for a resource while the `hard` limit acts as a ceiling for that value that could be set by an unprivileged process.

###### Example

```json
   "rlimits": [
        {
            "type": "RLIMIT_NPROC",
            "soft": 1024,
            "hard": 102400
        }
   ]
```

## SELinux process label

SELinux process label specifies the label with which the processes in a container are run.
For more information about SELinux, see  [Selinux documentation](http://selinuxproject.org/page/Main_Page)

###### Example

```json
   "selinuxProcessLabel": "system_u:system_r:svirt_lxc_net_t:s0:c124,c675"
```

## Apparmor profile

Apparmor profile specifies the name of the apparmor profile that will be used for the container.
For more information about Apparmor, see [Apparmor documentation](https://wiki.ubuntu.com/AppArmor)

###### Example

```json
   "apparmorProfile": "acme_secure_profile"
```

## seccomp

Seccomp provides application sandboxing mechanism in the Linux kernel.
Seccomp configuration allows one to configure actions to take for matched syscalls and furthermore also allows matching on values passed as arguments to syscalls.
For more information about Seccomp, see [Seccomp kernel documentation](https://www.kernel.org/doc/Documentation/prctl/seccomp_filter.txt)
The actions, architectures, and operators are strings that match the definitions in seccomp.h from [libseccomp](https://github.com/seccomp/libseccomp) and are translated to corresponding values.
A valid list of constants as of Libseccomp v2.2.3 is contained below.

Architecture Constants
* `SCMP_ARCH_X86`
* `SCMP_ARCH_X86_64`
* `SCMP_ARCH_X32`
* `SCMP_ARCH_ARM`
* `SCMP_ARCH_AARCH64`
* `SCMP_ARCH_MIPS`
* `SCMP_ARCH_MIPS64`
* `SCMP_ARCH_MIPS64N32`
* `SCMP_ARCH_MIPSEL`
* `SCMP_ARCH_MIPSEL64`
* `SCMP_ARCH_MIPSEL64N32`

Action Constants:
* `SCMP_ACT_KILL`
* `SCMP_ACT_TRAP`
* `SCMP_ACT_ERRNO`
* `SCMP_ACT_TRACE`
* `SCMP_ACT_ALLOW`

Operator Constants:
* `SCMP_CMP_NE`
* `SCMP_CMP_LT`
* `SCMP_CMP_LE`
* `SCMP_CMP_EQ`
* `SCMP_CMP_GE`
* `SCMP_CMP_GT`
* `SCMP_CMP_MASKED_EQ`

###### Example

```json
   "seccomp": {
       "defaultAction": "SCMP_ACT_ALLOW",
       "architectures": [
           "SCMP_ARCH_X86"
       ],
       "syscalls": [
           {
               "name": "getcwd",
               "action": "SCMP_ACT_ERRNO"
           }
       ]
   }
```

## Rootfs Mount Propagation

rootfsPropagation sets the rootfs's mount propagation.
Its value is either slave, private, or shared.
[The kernel doc](https://www.kernel.org/doc/Documentation/filesystems/sharedsubtree.txt) has more information about mount propagation.

###### Example

```json
    "rootfsPropagation": "slave",
```

## No new privileges

Setting `noNewPrivileges` to true prevents the processes in the container from gaining additional privileges.
[The kernel doc](https://www.kernel.org/doc/Documentation/prctl/no_new_privs.txt) has more information on how this is achieved using a prctl system call.

###### Example

```json
    "noNewPrivileges": true,
```

[cgroup-v1]: https://www.kernel.org/doc/Documentation/cgroup-v1/cgroups.txt
[cgroup-v1-blkio]: https://www.kernel.org/doc/Documentation/cgroup-v1/blkio-controller.txt
[cgroup-v1-cpusets]: https://www.kernel.org/doc/Documentation/cgroup-v1/cpusets.txt
[cgroup-v1-devices]: https://www.kernel.org/doc/Documentation/cgroup-v1/devices.txt
[cgroup-v1-hugetlb]: https://www.kernel.org/doc/Documentation/cgroup-v1/hugetlb.txt
[cgroup-v1-memory]: https://www.kernel.org/doc/Documentation/cgroup-v1/memory.txt
[cgroup-v1-net-cls]: https://www.kernel.org/doc/Documentation/cgroup-v1/net_cls.txt
[cgroup-v1-net-prio]: https://www.kernel.org/doc/Documentation/cgroup-v1/net_prio.txt
[cgroup-v1-pids]: https://www.kernel.org/doc/Documentation/cgroup-v1/pids.txt
[cgroup-v2]: https://www.kernel.org/doc/Documentation/cgroup-v2.txt
[devices]: https://www.kernel.org/doc/Documentation/devices.txt
[devpts]: https://www.kernel.org/doc/Documentation/filesystems/devpts.txt

[mknod.1]: http://man7.org/linux/man-pages/man1/mknod.1.html
[mknod.2]: http://man7.org/linux/man-pages/man2/mknod.2.html
[console.4]: http://man7.org/linux/man-pages/man4/console.4.html
[full.4]: http://man7.org/linux/man-pages/man4/full.4.html
[null.4]: http://man7.org/linux/man-pages/man4/null.4.html
[pts.4]: http://man7.org/linux/man-pages/man4/pts.4.html
[random.4]: http://man7.org/linux/man-pages/man4/random.4.html
[tty.4]: http://man7.org/linux/man-pages/man4/tty.4.html
[zero.4]: http://man7.org/linux/man-pages/man4/zero.4.html
