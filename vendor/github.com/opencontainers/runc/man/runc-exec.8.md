# NAME
   runc exec - execute new process inside the container

# SYNOPSIS
   runc exec [command options] <container-id> <container command>

Where "<container-id>" is the name for the instance of the container and
"<container command>" is the command to be executed in the container.

# EXAMPLE
For example, if the container is configured to run the linux ps command the
following will output a list of processes running in the container:

       # runc exec <container-id> ps

# OPTIONS
   --console                                    specify the pty slave path for use with the container
   --cwd                                        current working directory in the container
   --env, -e [--env option --env option]        set environment variables
   --tty, -t                                    allocate a pseudo-TTY
   --user, -u                                   UID (format: <uid>[:<gid>])
   --process, -p                                path to the process.json
   --detach, -d                                 detach from the container's process
   --pid-file                                   specify the file to write the process id to
   --process-label                              set the asm process label for the process commonly used with selinux
   --apparmor                                   set the apparmor profile for the process
   --no-new-privs                               set the no new privileges value for the process
   --cap, -c [--cap option --cap option]        add a capability to the bounding set for the process
