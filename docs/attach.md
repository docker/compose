# Attaching to STDIO or TTY

The model for STDIO, TTY, and logging is a little different in containerd.
Because of the various methods that consumers want on the logging side these types of decisions 
are pushed to the client.
Containerd API is developed for access on a single host therefore many things like paths on the host system are acceptable in the API.
For the STDIO model the client requesting to start a container provides the paths for the IO.

## Logging

If no options are specified on create all STDIO of the processes launched by containerd will be sent to `/dev/null`.
If you want containerd to send the STDIO of the processes to a file, you can pass paths to the files in the create container method defined by this proto in the stdin, stdout, and stderr fields:

```proto
message CreateContainerRequest {
	string id = 1; // ID of container
	string bundlePath = 2; // path to OCI bundle
	string stdin = 3; // path to the file where stdin will be read (optional)
	string stdout = 4; // path to file where stdout will be written (optional)
	string stderr = 5; // path to file where stderr will be written (optional)
	string console = 6; // path to the console for a container (optional)
	string checkpoint = 7; // checkpoint name if you want to create immediate checkpoint (optional)
}
```

## Attach

In order to have attach like functionality for your containers you use the same API request but named pipes or fifos can be used to achieve this type of functionality.
The default CLI for containerd does this if you specify the `--attach` flag on `create` or `start`.
It will create fifos for each of the containers stdio which the CLI can read and write to.
This can be used to create an interactive session with the container, `bash` for example, or to have a blocking way to collect the container's STDIO and forward it to your logging facilities.

## TTY

The tty model is the same as above only the client creates a pty and provides to other side to containerd in the create request in the `console` field.
Containerd will provide the pty to the container to use and the session can be opened with the container after it starts.
