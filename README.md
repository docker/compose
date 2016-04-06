# containerd

Containerd is a daemon to control runC, built for performance and density. 
Containerd leverages runC's advanced features such as seccomp and user namespace support as well
as checkpoint and restore for cloning and live migration of containers.

## Getting started

The easiest way to start using containerd is to download binaries from the [releases page](https://github.com/docker/containerd/releases).

The included `ctr` command-line tool allows you interact with the containerd daemon:

```
$ sudo ctr containers start redis /containers/redis
$ sudo ctr containers list
ID                  PATH                STATUS              PROCESSES
1                   /containers/redis   running             14063
```

`/containers/redis` is the path to an OCI bundle. [See the docs for more information.](docs/bundle.md)

## Docs

 * [Client CLI reference (`ctr`)](docs/cli.md)
 * [Daemon CLI reference (`containerd`)](docs/daemon.md)
 * [Creating OCI bundles](docs/bundle.md)
 * [containerd changes to the bundle](docs/bundle-changes.md)
 * [Attaching to STDIO or TTY](docs/attach.md)
 * [Telemetry and metrics](docs/telemetry.md)

All documentation is contained in the `/docs` directory in this repository.

## Building

You will need to make sure that you have Go installed on your system and the containerd repository is cloned
in your `$GOPATH`.  You will also need to make sure that you have all the dependencies cloned as well.
Currently, contributing to containerd is not for the first time devs as many dependencies are not vendored and 
work is being completed at a high rate.  

After that just run `make` and the binaries for the daemon and client will be localed in the `bin/` directory.

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

## Roadmap

The current roadmap and milestones for alpha and beta completion are in the github issues on this repository.  Please refer to these issues for what is being worked on and completed for the various stages of development.

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

Copyright © 2016 Docker, Inc. All rights reserved, except as follows. Code
is released under the Apache 2.0 license. The README.md file, and files in the
"docs" folder are licensed under the Creative Commons Attribution 4.0
International License under the terms and conditions set forth in the file
"LICENSE.docs". You may obtain a duplicate copy of the same license, titled
CC-BY-SA-4.0, at http://creativecommons.org/licenses/by/4.0/.
