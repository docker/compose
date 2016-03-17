# NAME
   runc kill - kill sends the specified signal (default: SIGTERM) to the container's init process

# SYNOPSIS
   runc kill <container-id> <signal>

Where "<container-id>" is the name for the instance of the container and
"<signal>" is the signal to be sent to the init process.

# EXAMPLE

For example, if the container id is "ubuntu01" the following will send a "KILL"
signal to the init process of the "ubuntu01" container:

       # runc kill ubuntu01 KILL
