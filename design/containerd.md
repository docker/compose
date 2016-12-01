# containerd

containerd is built to be a high performance container runtime able to support containers, processes, images, and networking primitives for supporting high level platforms.

Some of the design considerations for containerd are as follows:

* High performance
* Light on resources
* Expose internal metrics
* Comprised of multiple loosely coupled components   
* Able to be upgraded without impact to running containers
* Restore from crashes


## Design 

Below is the high level design of the daemon and its components:

