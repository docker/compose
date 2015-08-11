<!--[metadata]>
+++
title = "Command Completion"
description = "Compose CLI reference"
keywords = ["fig, composition, compose, docker, orchestration, cli,  reference"]
[menu.main]
parent="smn_workw_compose"
weight=3
+++
<![end-metadata]-->

# Command Completion

Compose comes with [command completion](http://en.wikipedia.org/wiki/Command-line_completion)
for the bash and zsh shell.

## Installing Command Completion

### Bash

Make sure bash completion is installed. If you use a current Linux in a non-minimal installation, bash completion should be available.
On a Mac, install with `brew install bash-completion`

Place the completion script in `/etc/bash_completion.d/` (`/usr/local/etc/bash_completion.d/` on a Mac), using e.g.

     curl -L https://raw.githubusercontent.com/docker/compose/$(docker-compose --version | awk 'NR==1{print $NF}')/contrib/completion/bash/docker-compose > /etc/bash_completion.d/docker-compose

Completion will be available upon next login.

### Zsh

Place the completion script in your `/path/to/zsh/completion`, using e.g. `~/.zsh/completion/`

    mkdir -p ~/.zsh/completion
    curl -L https://raw.githubusercontent.com/docker/compose/$(docker-compose --version | awk 'NR==1{print $NF}')/contrib/completion/zsh/_docker-compose > ~/.zsh/completion/_docker-compose

Include the directory in your `$fpath`, e.g. by adding in `~/.zshrc`

    fpath=(~/.zsh/completion $fpath)

Make sure `compinit` is loaded or do it by adding in `~/.zshrc`

    autoload -Uz compinit && compinit -i

Then reload your shell

    exec $SHELL -l

## Available completions

Depending on what you typed on the command line so far, it will complete

 - available docker-compose commands
 - options that are available for a particular command
 - service names that make sense in a given context (e.g. services with running or stopped instances or services based on images vs. services based on Dockerfiles). For `docker-compose scale`, completed service names will automatically have "=" appended.
 - arguments for selected options, e.g. `docker-compose kill -s` will complete some signals like SIGHUP and SIGUSR1.

Enjoy working with Compose faster and with less typos!

## Compose documentation

- [User guide](/)
- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Command line reference](/reference)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
