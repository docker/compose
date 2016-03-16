# NAME
   runc checkpoint - checkpoint a running container

# USAGE
   runc checkpoint [command options] <container-id>

Where "<container-id>" is the name for the instance of the container to be
checkpointed.

# DESCRIPTION
   The checkpoint command saves the state of the container instance.

# OPTIONS
   --image-path                 path for saving criu image files
   --work-path                  path for saving work files and logs
   --leave-running              leave the process running after checkpointing
   --tcp-established            allow open tcp connections
   --ext-unix-sk                allow external unix sockets
   --shell-job                  allow shell jobs
   --page-server                ADDRESS:PORT of the page server
   --file-locks                 handle file locks, for safety
   --manage-cgroups-mode        cgroups mode: 'soft' (default), 'full' and 'strict'.
