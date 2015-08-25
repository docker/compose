<!--[metadata]>
+++
title = "Docker Compose"
description = "How to install Docker Compose"
keywords = ["compose, orchestration, install, installation, docker, documentation"]
[menu.main]
parent="mn_install"
weight=4
+++
<![end-metadata]-->


# Install Docker Compose

You can run Compose on OS X and 64-bit Linux.  It is currently not supported on
the Windows operating system. To install Compose, you'll need to install Docker
first.

Depending on how your system is configured, you may require `sudo` access to
install Compose. If your system requires `sudo`, you will receive "Permission
denied" errors when installing Compose. If this is the case for you, preface the
install commands with `sudo` to install.

To install Compose, do the following:

1. Install Docker Engine version 1.7.1 or greater:

     * <a href="https://docs.docker.com/installation/mac/" target="_blank">Mac OS X installation</a> (installs both Engine and Compose)

     * <a href="https://docs.docker.com/installation/ubuntulinux/" target="_blank">Ubuntu installation</a>

     * <a href="https://docs.docker.com/installation/" target="_blank">other system installations</a>

2. Mac OS X users are done installing. Others should continue to the next step.

3. Go to the <a href="https://github.com/docker/compose/releases" target="_blank">repository release page</a>.

4. Enter the `curl` command in your termial.

     The command has the following format:

        curl -L https://github.com/docker/compose/releases/download/VERSION_NUM/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose

     If you have problems installing with `curl`, you can use `pip` instead: `pip install -U docker-compose`

4. Apply executable permissions to the binary:

        $ chmod +x /usr/local/bin/docker-compose

5.  Optionally, install [command completion](completion.md) for the
`bash` and `zsh` shell.

6. Test the installation.

        $ docker-compose --version
        docker-compose version: 1.4.0

## Upgrading

If you're upgrading from Compose 1.2 or earlier, you'll need to remove or migrate
your existing containers after upgrading Compose. This is because, as of version
1.3, Compose uses Docker labels to keep track of containers, and so they need to
be recreated with labels added.

If Compose detects containers that were created without labels, it will refuse
to run so that you don't end up with two sets of them. If you want to keep using
your existing containers (for example, because they have data volumes you want
to preserve) you can migrate them with the following command:

    $ docker-compose migrate-to-labels

Alternatively, if you're not worried about keeping them, you can remove them &endash;
Compose will just create new ones.

    $ docker rm -f -v myapp_web_1 myapp_db_1 ...


## Uninstallation

To uninstall Docker Compose if you installed using `curl`:

    $ rm /usr/local/bin/docker-compose


To uninstall Docker Compose if you installed using `pip`:

    $ pip uninstall docker-compose

>**Note**: If you get a "Permission denied" error using either of the above
>methods, you probably do not have the proper permissions to remove
>`docker-compose`.  To force the removal, prepend `sudo` to either of the above
>commands and run again.


## Where to go next

- [User guide](/)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Command line reference](/reference)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
