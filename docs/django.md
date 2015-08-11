<!--[metadata]>
+++
title = "Quickstart Guide: Compose and Django"
description = "Getting started with Docker Compose and Django"
keywords = ["documentation, docs,  docker, compose, orchestration, containers"]
[menu.main]
parent="smn_workw_compose"
weight=4
+++
<![end-metadata]-->


## Quickstart Guide: Compose and Django


This Quick-start Guide will demonstrate how to use Compose to set up and run a
simple Django/PostgreSQL app. Before starting, you'll need to have
[Compose installed](install.md).

### Define the project

Start by setting up the three files you'll need to build the app. First, since
your app is going to run inside a Docker container containing all of its
dependencies, you'll need to define exactly what needs to be included in the
container. This is done using a file called `Dockerfile`. To begin with, the
Dockerfile consists of:

    FROM python:2.7
    ENV PYTHONUNBUFFERED 1
    RUN mkdir /code
    WORKDIR /code
    ADD requirements.txt /code/
    RUN pip install -r requirements.txt
    ADD . /code/

This Dockerfile will define an image that is used to build a container that
includes your application and has Python installed alongside all of your Python
dependencies. For more information on how to write Dockerfiles, see the
[Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

Second, you'll define your Python dependencies in a file called
`requirements.txt`:

    Django
    psycopg2

Finally, this is all tied together with a file called `docker-compose.yml`. It
describes the services that comprise your app (here, a web server and database),
which Docker images they use, how they link together, what volumes will be
mounted inside the containers, and what ports they expose.

    db:
      image: postgres
    web:
      build: .
      command: python manage.py runserver 0.0.0.0:8000
      volumes:
        - .:/code
      ports:
        - "8000:8000"
      links:
        - db

See the [`docker-compose.yml` reference](yml.md) for more information on how
this file works.

### Build the project

You can now start a Django project with `docker-compose run`:

    $ docker-compose run web django-admin.py startproject composeexample .

First, Compose will build an image for the `web` service using the `Dockerfile`.
It will then run `django-admin.py startproject composeexample .` inside a
container built using that image.

This will generate a Django app inside the current directory:

    $ ls
    Dockerfile       docker-compose.yml          composeexample       manage.py        requirements.txt

### Connect the database

Now you need to set up the database connection. Replace the `DATABASES = ...`
definition in `composeexample/settings.py` to read:

    DATABASES = {
        'default': {
            'ENGINE': 'django.db.backends.postgresql_psycopg2',
            'NAME': 'postgres',
            'USER': 'postgres',
            'HOST': 'db',
            'PORT': 5432,
        }
    }

These settings are determined by the
[postgres](https://registry.hub.docker.com/_/postgres/) Docker image specified
in the Dockerfile.

Then, run `docker-compose up`:

    Recreating myapp_db_1...
    Recreating myapp_web_1...
    Attaching to myapp_db_1, myapp_web_1
    myapp_db_1 |
    myapp_db_1 | PostgreSQL stand-alone backend 9.1.11
    myapp_db_1 | 2014-01-27 12:17:03 UTC LOG:  database system is ready to accept connections
    myapp_db_1 | 2014-01-27 12:17:03 UTC LOG:  autovacuum launcher started
    myapp_web_1 | Validating models...
    myapp_web_1 |
    myapp_web_1 | 0 errors found
    myapp_web_1 | January 27, 2014 - 12:12:40
    myapp_web_1 | Django version 1.6.1, using settings 'composeexample.settings'
    myapp_web_1 | Starting development server at http://0.0.0.0:8000/
    myapp_web_1 | Quit the server with CONTROL-C.

Your Django app should nw be running at port 8000 on your Docker daemon. If you are using a Docker Machine VM, you can use the `docker-machine ip MACHINE_NAME` to get the IP address.

You can also run management commands with Docker. To set up your database, for
example, run `docker-compose up` and in another terminal run:

    $ docker-compose run web python manage.py syncdb

## More Compose documentation

- [User guide](/)
- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Command line reference](/reference)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
