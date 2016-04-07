# Daemon options

```
$ containerd -h

NAME:
   containerd - High performance container daemon

USAGE:
   containerd [global options] command [command options] [arguments...]

VERSION:
   0.1.0 commit: 54c213e8a719d734001beb2cb8f130c84cc3bd20

COMMANDS:
   help, h      Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --debug                                              enable debug output in the logs
   --state-dir "/run/containerd"                        runtime state directory
   --metrics-interval "5m0s"                            interval for flushing metrics to the store
   --listen, -l "/run/containerd/containerd.sock"       Address on which GRPC API will listen
   --runtime, -r "runc"                                 name of the OCI compliant runtime to use when executing containers
   --graphite-address                                   Address of graphite server
   --help, -h                                           show help
   --version, -v                                        print the version
```
