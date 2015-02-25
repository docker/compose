page_title: Installing Compose
page_description: How to intall Docker Compose
page_keywords: compose, orchestration, install, installation, docker, documentation


## Installing Compose

To install Compose, you'll need to install Docker first. You'll then install
Compose with a `curl` command. 

### Install Docker

First, you'll need to install Docker version 1.3 or greater.

If you're on OS X, you can use the
[OS X installer](https://docs.docker.com/installation/mac/) to install both
Docker and the OSX helper app, boot2docker. Once boot2docker is running, set the
environment variables that'll configure Docker and Compose to talk to it:

    $(boot2docker shellinit)

To persist the environment variables across shell sessions, add the above line
to your `~/.bashrc` file.

For complete instructions, or if you are on another platform, consult Docker's
[installation instructions](https://docs.docker.com/installation/).

### Install Compose

To install Compose, run the following commands:

    curl -L https://github.com/docker/compose/releases/download/1.1.0-rc2/docker-compose-`uname -s`-`uname -m` > /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose

Optionally, you can also install [command completion](completion.md) for the
bash shell.

Compose is available for OS X and 64-bit Linux. If you're on another platform,
Compose can also be installed as a Python package:

    $ sudo pip install -U docker-compose

No further steps are required; Compose should now be successfully  installed.
You can test the installation by running `docker-compose --version`.

## Compose documentation

- [User guide](index.md)
- [Command line reference](cli.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
