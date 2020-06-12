# Overview of the Docker command line

We try to make the Docker command line easy to use, even though you are dealing with a powerful tool and concepts that can be difficult.

## Getting help

In addition to this online manual, you can always ask for help from the command line itself. `docker help`, `docker --help` or `docker -h`
will list all the commands available. If you want to get help for a more specific command you can use `--help` or `-h` at any level,
for example `docker run --help` or `docker context create --help`.

## Build, share, run

Insert a really small tutorial or links here.

## The most important commands

- `docker run` will run a new container, from an image you or someone else has built
- `docker ps` will show the running containers
- `docker stop` will stop a running container
- `docker rm` will delete a stopped container NOTE may change this so you can rm a running container, why not?
- `docker build` builds a new container image from your source code
- `docker compose` manages a set of containers from a single Yaml configuration file

## Deprecated syntax

We have made some changes to the syntax of a few commands to make them easier to understand. Where we still support the old
forms, the command line will tell you the new form, but will still work correctly. In cases where we remove the old
form you will get help text. If we remove a verb, for example "docker stack" we will display a message saying that the command
is only available with the Local backend. For example

```
> docker context create my-context --description "some description" --docker "host=tcp://myserver:2376"
This form of the command is deprecated, please use
docker context create docker --description "some description" --host=tcp://myserver:2376
```
