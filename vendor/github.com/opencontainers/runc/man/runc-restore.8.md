# NAME
   runc restore - restore a container from a previous checkpoint

# SYNOPSIS
   runc restore [command options] <container-id>

Where "<container-id>" is the name for the instance of the container to be
restored.

# DESCRIPTION
   Restores the saved state of the container instance that was previously saved
using the runc checkpoint command.

# OPTIONS
   --image-path                 path to criu image files for restoring
   --work-path                  path for saving work files and logs
   --tcp-established            allow open tcp connections
   --ext-unix-sk                allow external unix sockets
   --shell-job                  allow shell jobs
   --file-locks                 handle file locks, for safety
   --manage-cgroups-mode        cgroups mode: 'soft' (default), 'full' and 'strict'.
   --bundle, -b                 path to the root of the bundle directory
   --detach, -d                 detach from the container's process
   --pid-file                   specify the file to write the process id to
