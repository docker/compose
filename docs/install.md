page_title: Installing Compose
page_description: How to install Docker Compose
page_keywords: compose, orchestration, install, installation, docker, documentation


## Installing Compose

To install Compose, you'll need to install Docker first. You'll then install
Compose with a `curl` command. 

### Install Docker

First, install Docker version 1.3 or greater:

- [Instructions for Mac OS X](http://docs.docker.com/installation/mac/)
- [Instructions for Ubuntu](http://docs.docker.com/installation/ubuntulinux/)
- [Instructions for other systems](http://docs.docker.com/installation/)

### Install Compose

To install Compose, run the following commands:

    curl -L https://github.com/docker/compose/releases/download/1.2.0/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose

> Note: If you get a "Permission denied" error, your `/usr/local/bin` directory probably isn't writable and you'll need to install Compose as the superuser. Run `sudo -i`, then the two commands above, then `exit`.

Optionally, you can also install [command completion](completion.md) for the
bash shell.

Compose is available for OS X and 64-bit Linux. If you're on another platform,
Compose can also be installed as a Python package:

    $ sudo pip install -U docker-compose

No further steps are required; Compose should now be successfully installed.
You can test the installation by running `docker-compose --version`.

## Compose documentation

- [User guide](index.md)
- [Command line reference](cli.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
