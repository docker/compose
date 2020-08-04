# docker run

Run will create a new container and execute commands in the specified image. An image is a packaged up filesystem that you have built
or that you pull from a registry such as Docker Hub.

You can either use run to create an interactive session, or run a container in the background and read the output or connect to it
later.

To create an interactive container, use `docker run --interactive --tty ...` or in short form `docker run -it ...`, for example
`docker run -it ubuntu` will give you an interactive session inside an Ubuntu image. To specify a command other than the default
for the image (which is to run `bash` in the case of Ubuntu), specify the command after the image name, for example
`docker run ubuntu ls`.

```
docker run [OPTIONS] _image_ [COMMAND] [ARG...]
```

## Options

```
      --name string                    Assign a name to the container; a default will be given otherwise
  -d, --detach                         Run container in background and print container ID
  -i, --interactive                    Keep STDIN open even if not attached
  -e, --env list                       Set environment variables
  -l, --label list                     Set metadata on a container
      --rm                             Automatically remove the container when it exits

  -w, --workdir string                 Working directory inside the container
  -h, --hostname string                Container host name
  -m, --memory bytes                   Memory limit
      --cpus                           Number of CPUs to allocate, approximately
  -p, --publish list                   Publish a container's port(s) to the host
  -P, --publish-all                    Publish all exposed ports to random ports
      --restart string                 Restart policy to apply when a container exits (default "none")
      --entrypoint string              Overwrite the default ENTRYPOINT of the image

      --mount mount                    Attach a filesystem mount to the container
  -v, --volume list                    Bind mount a volume

TODO net, profile, logger (d2 options) need to align with d2 and clouds
TODO I think --cpus is perhaps the best measure for eg clouds etc, but would need converting to cgroups measures for Linux.
```
