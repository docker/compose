# containerd

another container runtime

Start a container:

```bash
curl -XPOST localhost:8888/containers/redis -d '{"bundlePath": "/containers/redis"}' 
```

Add a process:

```bash
curl -s -XPUT localhost:8888/containers/redis/process -d@process.json | json_pp                   
{
   "pid" : 25671,
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
}
```


Get containers: 

```bash
curl -s localhost:8888/containers | json_pp
{
   "containers" : [
      {
         "processes" : [
            {
               "args" : [
                  "sh",
                  "-c",
                  "sleep 60"
               ],
               "user" : {
                  "gid" : 0,
                  "uid" : 0
               },
               "pid" : 25743,
               "env" : [
                  "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
                  "TERM=xterm"
               ]
            }
         ],
         "id" : "redis",
         "state" : {
            "status" : "running"
         },
         "bundlePath" : "/containers/redis"
      }
   ]
}

```

Other stuff:

```bash
# pause and resume a container
curl -XPATCH localhost:8888/containers/redis -d '{"status": "paused"}'
curl -XPATCH localhost:8888/containers/redis -d '{"status": "running"}'

# send signal to a container's specific process
curl -XPOST localhost:8888/containers/redis/process/18306 -d '{"signal": 9}'
```
