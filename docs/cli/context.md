# docker context

A context is a way of easily referring to a particular place that you want to run containers. You can set up a context and refer to
it by name, and easily switch between different contexts, so you can use containers in different places.

The most important commands for working with a context are
- `docker context create` will create a new context from options specified on the command line
- `docker context rm` removes a context
- `docker context use` will set the context to use by default; you can also use `docker --context ...` or via the environment with `DOCKER_CONTEXT=... docker ...`
- `docker context show` shows the context name that you are currently using.
- `docker context ls` lists the available contexts
- `docker context login` some types of context require credentials that may expire, such as OAuth credentials. If the credentials have
expired you will need to use `docker context login` to refresh them.

In addition you can use
- `docker context inspect` to see the full details of a context
- `docker context import` to import a context NOTE may make changes here, why can't create create from a file etc?
- `docker context export` to output a context NOTE may make changes here

## docker context create

To create a new context from options specified on the command line use `docker context create`. The exact information you need
to create a context depends on the particular backend.

NOTE for compatibility we may need to support existing syntax as well, but this should be deprecated. This may be a case where there are few users so can just change it.

Usage:
```
docker context create _name_ _type_ [options]
docker context create _name_ url
```
NOTE we could also use `docker context create _type_ _name_` might be more consistent, makes the url form less clear.


```
docker context create "myserver" docker --description "some description" --host=tcp://myserver:2376
```

In addition there is a URL form for contexts which is a little harder to read but lets you share contexts more easily.

```
docker context create "myserver" docker://myserver:2376?type=org.moby.moby
```
NOTE: really don't think the URL form will work for the current mutual TLS auth, without extremely long URLs. Not important.

## docker context use

Once you have created a context with `docker context create`, then you have given it a name. You can switch to the context with
`docker context use _name_`. This will be used for all Docker commands from then on, unless you specify the context explicitly
with `docker --context _name_ ...` or use the `DOCKER_CONTEXT` environment variable.

## docker context show

To see what the current context is, use `docker context show`.

NOTE: this is new, its kind of weird there is no way to easily display name of current context without parsing output of
`docker context inspect`, eg for use in shell prompt. We could also use plain `docker context` but this is not a pattern we use
much elsewhere so TBD.

## docker context ls

This lists all the currently configured contexts. Use `docker context ls -q` or `docker context ls --quiet` to just list the names
not the descriptions.

## docker context login

Some contexts have a login that will expire, for example if they use OAuth authentication. In this case, trying to use a context will
give an error that you are not authenticated. Use `docker context login` to log in to the current context, and then follow the prompts.

## TODO context sharing

`docker context pull`? to get shared context?
`docker context send justincormack'? to send to justin via Hub.
