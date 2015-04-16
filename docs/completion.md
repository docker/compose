---
layout: default
title: Command Completion
---

Command Completion
==================

Compose comes with [command completion](http://en.wikipedia.org/wiki/Command-line_completion)
for the bash shell.

Installing Command Completion
-----------------------------

Make sure bash completion is installed. If you use a current Linux in a non-minimal installation, bash completion should be available.
On a Mac, install with `brew install bash-completion`
 
Place the completion script in `/etc/bash_completion.d/` (`/usr/local/etc/bash_completion.d/` on a Mac), using e.g. 

     curl -L https://raw.githubusercontent.com/docker/compose/1.2.0/contrib/completion/bash/docker-compose > /etc/bash_completion.d/docker-compose
 
Completion will be available upon next login.

Available completions
---------------------
Depending on what you typed on the command line so far, it will complete

 - available docker-compose commands
 - options that are available for a particular command
 - service names that make sense in a given context (e.g. services with running or stopped instances or services based on images vs. services based on Dockerfiles). For `docker-compose scale`, completed service names will automatically have "=" appended.
 - arguments for selected options, e.g. `docker-compose kill -s` will complete some signals like SIGHUP and SIGUSR1.

Enjoy working with Compose faster and with less typos!

## Compose documentation

- [Installing Compose](install.md)
- [User guide](index.md)
- [Command line reference](cli.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
