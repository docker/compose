# containerd changes to the bundle

Containerd will make changes to the container's bundle by adding additional files or folders by default with
options to change the output.  

The current change that it makes is if you create a checkpoint of a container, the checkpoints will be saved
by default in the container bundle at `{bundle}/checkpoints/{checkpoint name}`.
A user can also populate this directory and provide the checkpoint name on the create request so that the container is started from this checkpoint.  


As of this point, containerd has no other additions to the bundle.
Runtime state is currently stored in a tmpfs filesystem like `/run`.
