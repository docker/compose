# NAME
   runc start - create and run a container

# SYNOPSIS
   runc start [command options] <container-id>

Where "<container-id>" is your name for the instance of the container that you
are starting. The name you provide for the container instance must be unique on
your host.

# DESCRIPTION
   The start command creates an instance of a container for a bundle. The bundle
is a directory with a specification file and a root filesystem.

# OPTIONS
   --bundle, -b         path to the root of the bundle directory, defaults to the current directory
   --console            specify the pty slave path for use with the container
   --detach, -d         detach from the container's process
   --pid-file           specify the file to write the process id to
