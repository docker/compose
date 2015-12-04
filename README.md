# containerd

Another container runtime built for performance and density. 
containerd has advanced features such as seccomp and user namespace support as well
as checkpoint and restore for cloning and live migration of containers.

#### Status 

*alpha*

What does alpha, beta, etc mean?
* alpha - not feature complete
* beta - feature complete but needs testing
* prod ready - read for production


# Performance

Starting 1000 containers concurrently runs at 126-140 containers per second.

Overall start times:

```
[containerd] 2015/12/04 15:00:54   count:        1000
[containerd] 2015/12/04 14:59:54   min:          23ms
[containerd] 2015/12/04 14:59:54   max:         355ms
[containerd] 2015/12/04 14:59:54   mean:         78ms
[containerd] 2015/12/04 14:59:54   stddev:       34ms
[containerd] 2015/12/04 14:59:54   median:       73ms
[containerd] 2015/12/04 14:59:54   75%:          91ms
[containerd] 2015/12/04 14:59:54   95%:         123ms
[containerd] 2015/12/04 14:59:54   99%:         287ms
[containerd] 2015/12/04 14:59:54   99.9%:       355ms
```


# REST API v1

## Starting a container

To start a container hit the `/containers/{name}` endpoint with a `POST` request.
The checkpoint field is option but allows you to start the container with the specified
checkpoint name instead of a new instance of the container.

Example:
```bash
curl -XPOST localhost:8888/containers/redis -d '{
    "bundlePath": "/containers/redis",
    "checkpoint: "checkpoint-name"
    }' 
```

## Add a process to an existing container

To add an additional process to a running container send a `PUT` request to the
`/containers/{name}/processes` endpoint.

Example:
```bash
curl -s -XPUT localhost:8888/containers/redis/process -d '{
       "user" : {
          "gid" : 0,
          "uid" : 0
       },
       "args" : [
          "sh",
          "-c",
          "sleep 10"
       ],
       "env" : [
          "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
          "TERM=xterm"
       ]
    }'
```


## Signal a specific process in a container

To send a signal to any of the containers processes send a  `POST` request to
the `/containers/{name}/process/{pid}` endpoint.

Example

```bash
curl -s -XPOST  localhost:8888/containers/redis/process/1234 -d '{"signal": 15}'
```

## Get the state of containerd and all of its containers

To the the entire state of the containerd instance send a `GET` request
to the `/state` endpoint.

Example:
```bash
curl -s localhost:8888/state
```

Response:
```json
{
   "containers" : [
      {
         "state" : {
            "status" : "running"
         },
         "bundlePath" : "/containers/redis",
         "id" : "redis",
         "processes" : [
            {
               "args" : [
                  "redis-server",
                  "--bind",
                  "0.0.0.0"
               ],
               "user" : {
                  "gid" : 1000,
                  "uid" : 1000
               },
               "terminal" : false,
               "pid" : 11497,
               "env" : [
                  "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                  "TERM=xterm"
               ]
            }
         ]
      }
   ],
   "machine" : {
      "cpus" : 4,
      "memory" : 7872909312
   }
}
```

## Create a checkpoint for a container

To create a checkpoint for a container send a `POST` request to the 
`/containers/{name}/checkpoint/{checkpointname}` endpoint.  All of the options
to this endpoint are optional.

If you send `"exit": true` the container will be stopped after the checkpoint is complete,
the default is to keep the container running.

Example:

```bash
curl -s -XPOST localhost:8888/containers/redis/checkpoint/test1 -d '{
        "exit": false,
        "tcp": false,
        "unixSockets": false,
        "shell": false
    }'
```

## List all checkpoints for a container

To list all checkpoints for a container send a `GET` request to the 
`/containers/{name}/checkpoint` endpoint.

Example:

```bash
curl -s localhost:8888/containers/redis/checkpoint
```

Response:

```json
[
   {
      "name" : "test1",
      "unixSockets" : false,
      "tcp" : false,
      "shell" : false
   },
   {
      "name" : "test2",
      "tcp" : false,
      "unixSockets" : false,
      "shell" : false
   }
]
```

## Delete a container's checkpoint

To delete a container's checkpoint send a `DELETE` request to the 
`/containers/redis/checkpoint/{checkpointname}` endpoint.

Example:

```bash
curl -XDELETE -s localhost:8888/containers/redis/checkpoint/test1
```

## Update a container

The update endpoint for a container accepts a JSON object with various fields 
for the actions you with to perform.  To update a container send a `PATCH` request
to the `/containers/{name}` endpoint.

### Pause and resume a container

To pause or resume a continer you want to send a `PATCH` request updating the container's state.

Example:

```bash
# pause a container
curl -XPATCH localhost:8888/containers/redis -d '{"status": "paused"}'

# resume the container
curl -XPATCH localhost:8888/containers/redis -d '{"status": "running"}'
```

### Signal the main process of a container

To signal the main process of the container hit the same update endpoint with a different state.

Example:

```bash
curl -s -XPATCH localhost:8888/containers/redis -d '{"signal": 9}'
```
