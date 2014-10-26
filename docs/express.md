---
layout: default
title: Getting started with Fig and Express
---

Getting started with Fig and Express
==================================

We're going to use Fig to set up and run a Node.js Express/Redis app. Before starting, you'll need to have [Fig installed](install.html).

You'll want to make a directory for the project:

    $ mkdir figexpress
    $ cd figexpress

Inside this directory, create `app.js`, a simple web app that uses the Express framework and increments a value in Redis:

```js
var express = require('express');
var app = express();
var redis = require("redis");
var client = redis.createClient(6379,'redis',{});

app.get('/', function(req, res){
  client.incr('hits', function (){
    client.get('hits', function (err, data) {
      res.send('Hello World! I have been seen ' + data + ' times.');
    });
  });
});

app.listen(3000);
```

Now let's create a simple `package.json` file for our app specifying our dependencies:

```js
{
  "name": "helloworld",
  "version": "0.0.0",
  "description": "example for fig",
  "private": "true",
  "main": "app.js",
  "dependencies": {
    "express": "^4.10.0",
    "redis": "^0.12.1"
  },
  "scripts": {
    "start": "node app.js"
  }
}
```

Define your app's environment with a `Dockerfile` so it can be reproduced anywhere. We're going to base our image off of dockerfile/nodejs:

    FROM dockerfile/nodejs
    ADD . /code
    WORKDIR /code
    RUN npm install
    ENTRYPOINT npm start

For more information on how to write Dockerfiles, see the [Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

Define the services that make up your app in `fig.yml` so they can be run together in an isolated environment:

```yaml
web:
  build: .
  ports:
    - "3000:3000"
  links:
    - redis
redis:
  image: redis
```

Now if we run `fig up`, it'll pull a Redis image, build an image for our own code, and start everything up:

    $ fig up

If all's well, you should see Redis startup listening to port 6379 and node app.js startup:

    redis_1 | [1] 26 Oct 18:31:09.775 * The server is now ready to accept connections on port 6379
    web_1   | 
    web_1   | > helloworld@0.0.0 start /code
    web_1   | > node app.js
    web_1   | 

The web app should now be listening on port 3000 on your docker daemon:

    $ curl localhost:3000
    Hello World! I have been seen 1 times.


