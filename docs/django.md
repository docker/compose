---
layout: default
title: Getting started with Fig and Django
---

Getting started with Fig and Django
===================================

Let's use Fig to set up and run a Django/PostgreSQL app. Before starting, you'll need to have [Fig installed](install.html).

Let's set up the three files that'll get us started. First, our app is going to be running inside a Docker container which contains all of its dependencies. We can define what goes inside that Docker container using a file called `Dockerfile`. It'll contain this to start with:

    FROM orchardup/python:2.7
    ENV PYTHONUNBUFFERED 1
    RUN apt-get update -qq && apt-get install -y python-psycopg2
    RUN mkdir /code
    WORKDIR /code
    ADD requirements.txt /code/
    RUN pip install -r requirements.txt
    ADD . /code/

That'll install our application inside an image with Python installed alongside all of our Python dependencies. For more information on how to write Dockerfiles, see the [Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

Second, we define our Python dependencies in a file called `requirements.txt`:

    Django

Simple enough. Finally, this is all tied together with a file called `fig.yml`. It describes the services that our app comprises of (a web server and database), what Docker images they use, how they link together, what volumes will be mounted inside the containers and what ports they expose.

    db:
      image: orchardup/postgresql
    web:
      build: .
      command: python manage.py runserver 0.0.0.0:8000
      volumes:
        - .:/code
      ports:
        - "8000:8000"
      links:
        - db

See the [`fig.yml` reference](http://orchardup.github.io/fig/yml.html) for more information on how it works.

We can now start a Django project using `fig run`:

    $ fig run web django-admin.py startproject figexample .

First, Fig will build an image for the `web` service using the `Dockerfile`. It will then run `django-admin.py startproject figexample .` inside a container using that image.

This will generate a Django app inside the current directory:

    $ ls
    Dockerfile       fig.yml          figexample       manage.py        requirements.txt

First thing we need to do is set up the database connection. Replace the `DATABASES = ...` definition in `figexample/settings.py` to read:

    DATABASES = {
        'default': {
            'ENGINE': 'django.db.backends.postgresql_psycopg2',
            'NAME': 'docker',
            'USER': 'docker',
            'PASSWORD': 'docker',
            'HOST': os.environ.get('DB_1_PORT_5432_TCP_ADDR'),
            'PORT': os.environ.get('DB_1_PORT_5432_TCP_PORT'),
        }
    }

These settings are determined by the [orchardup/postgresql](https://github.com/orchardup/docker-postgresql) Docker image we are using.

Then, run `fig up`:

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
    myapp_web_1 | Django version 1.6.1, using settings 'figexample.settings'
    myapp_web_1 | Starting development server at http://0.0.0.0:8000/
    myapp_web_1 | Quit the server with CONTROL-C.

And your Django app should be running at [localhost:8000](http://localhost:8000) (or [localdocker:8000](http://localdocker:8000) if you're using docker-osx).

You can also run management commands with Docker. To set up your database, for example, run `fig up` and in another terminal run:

    $ fig run web python manage.py syncdb

