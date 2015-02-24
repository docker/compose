Fig
==============

[![wercker status](https://app.wercker.com/status/d5dbac3907301c3d5ce735e2d5e95a5b/s/master "wercker status")](https://app.wercker.com/project/bykey/d5dbac3907301c3d5ce735e2d5e95a5b)

Fig is a tool for defining and running complex applications with Docker.
With Fig, you define a multi-container application in a single file, then
spin your application up in a single command which does everything that needs to
be done to get it running.

Fig is great for development environments, staging servers, and CI. We don't
recommend that you use it in production yet.

Using Fig is basically a three-step process.

First, you define your app's environment with a `Dockerfile` so it can be
reproduced anywhere:

```Dockerfile
FROM python:2.7
WORKDIR /code
ADD requirements.txt /code/
RUN pip install -r requirements.txt
ADD . /code
CMD python app.py
```

Next, you define the services that make up your app in `fig.yml` so
they can be run together in an isolated environment:

```yaml
web:
  build: .
  links:
   - db
  ports:
   - "8000:8000"
db:
  image: postgres
```

Lastly, run `fig up` and Fig will start and run your entire app.

Fig has commands for managing the whole lifecycle of your application:

 * Start, stop and rebuild services
 * View the status of running services
 * Stream the log output of running services
 * Run a one-off command on a service

Installation and documentation
------------------------------

Full documentation is available on [Fig's website](http://www.fig.sh/).
