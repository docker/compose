<!--[metadata]>
+++
title = "Quickstart Guide: Compose and Rails"
description = "Getting started with Docker Compose and Rails"
keywords = ["documentation, docs,  docker, compose, orchestration, containers"]
[menu.main]
parent="smn_workw_compose"
weight=5
+++
<![end-metadata]-->

## Quickstart Guide: Compose and Rails

This Quickstart guide will show you how to use Compose to set up and run a Rails/PostgreSQL app. Before starting, you'll need to have [Compose installed](install.md).

### Define the project

Start by setting up the three files you'll need to build the app. First, since
your app is going to run inside a Docker container containing all of its
dependencies, you'll need to define exactly what needs to be included in the
container. This is done using a file called `Dockerfile`. To begin with, the
Dockerfile consists of:

    FROM ruby:2.2.0
    RUN apt-get update -qq && apt-get install -y build-essential libpq-dev
    RUN mkdir /myapp
    WORKDIR /myapp
    ADD Gemfile /myapp/Gemfile
    RUN bundle install
    ADD . /myapp

That'll put your application code inside an image that will build a container with Ruby, Bundler and all your dependencies inside it. For more information on how to write Dockerfiles, see the [Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

Next, create a bootstrap `Gemfile` which just loads Rails. It'll be overwritten in a moment by `rails new`.

    source 'https://rubygems.org'
    gem 'rails', '4.2.0'

Finally, `docker-compose.yml` is where the magic happens. This file describes the services that comprise your app (a database and a web app), how to get each one's Docker image (the database just runs on a pre-made PostgreSQL image, and the web app is built from the current directory), and the configuration needed to link them together and expose the web app's port.

    db:
      image: postgres
    web:
      build: .
      command: bundle exec rails s -p 3000 -b '0.0.0.0'
      volumes:
        - .:/myapp
      ports:
        - "3000:3000"
      links:
        - db

### Build the project

With those three files in place, you can now generate the Rails skeleton app
using `docker-compose run`:

    $ docker-compose run web rails new . --force --database=postgresql --skip-bundle

First, Compose will build the image for the `web` service using the
`Dockerfile`. Then it'll run `rails new` inside a new container, using that
image. Once it's done, you should have generated a fresh app:

    $ ls
    Dockerfile   app          docker-compose.yml      tmp
    Gemfile      bin          lib          vendor
    Gemfile.lock config       log
    README.rdoc  config.ru    public
    Rakefile     db           test

Uncomment the line in your new `Gemfile` which loads `therubyracer`, so you've
got a Javascript runtime:

    gem 'therubyracer', platforms: :ruby

Now that you've got a new `Gemfile`, you need to build the image again. (This,
and changes to the Dockerfile itself, should be the only times you'll need to
rebuild.)

    $ docker-compose build

### Connect the database

The app is now bootable, but you're not quite there yet. By default, Rails
expects a database to be running on `localhost` - so you need to point it at the
`db` container instead. You also need to change the database and username to
align with the defaults set by the `postgres` image.

Open up your newly-generated `database.yml` file. Replace its contents with the
following:

    development: &default
      adapter: postgresql
      encoding: unicode
      database: postgres
      pool: 5
      username: postgres
      password:
      host: db

    test:
      <<: *default
      database: myapp_test

You can now boot the app with:

    $ docker-compose up

If all's well, you should see some PostgreSQL output, and then—after a few
seconds—the familiar refrain:

    myapp_web_1 | [2014-01-17 17:16:29] INFO  WEBrick 1.3.1
    myapp_web_1 | [2014-01-17 17:16:29] INFO  ruby 2.2.0 (2014-12-25) [x86_64-linux-gnu]
    myapp_web_1 | [2014-01-17 17:16:29] INFO  WEBrick::HTTPServer#start: pid=1 port=3000

Finally, you need to create the database. In another terminal, run:

    $ docker-compose run web rake db:create

That's it. Your app should now be running on port 3000 on your Docker daemon. If you're using [Docker Machine](https://docs.docker.com/machine), then `docker-machine ip MACHINE_VM` returns the Docker host IP address. 


## More Compose documentation

- [User guide](/)
- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Command line reference](cli.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
