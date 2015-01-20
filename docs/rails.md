---
layout: default
title: Getting started with Compose and Rails
---

Getting started with Compose and Rails
==================================

We're going to use Compose to set up and run a Rails/PostgreSQL app. Before starting, you'll need to have [Compose installed](install.html).

Let's set up the three files that'll get us started. First, our app is going to be running inside a Docker container which contains all of its dependencies. We can define what goes inside that Docker container using a file called `Dockerfile`. It'll contain this to start with:

    FROM ruby
    RUN apt-get update -qq && apt-get install -y build-essential libpq-dev
    RUN mkdir /myapp
    WORKDIR /myapp
    ADD Gemfile /myapp/Gemfile
    RUN bundle install
    ADD . /myapp

That'll put our application code inside an image with Ruby, Bundler and all our dependencies. For more information on how to write Dockerfiles, see the [Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

Next, we have a bootstrap `Gemfile` which just loads Rails. It'll be overwritten in a moment by `rails new`.

    source 'https://rubygems.org'
    gem 'rails', '4.0.2'

Finally, `docker-compose.yml` is where the magic happens. It describes what services our app comprises (a database and a web app), how to get each one's Docker image (the database just runs on a pre-made PostgreSQL image, and the web app is built from the current directory), and the condocker-composeuration we need to link them together and expose the web app's port.

    db:
      image: postgres
      ports:
        - "5432"
    web:
      build: .
      command: bundle exec rackup -p 3000
      volumes:
        - .:/myapp
      ports:
        - "3000:3000"
      links:
        - db

With those files in place, we can now generate the Rails skeleton app using `docker-compose run`:

    $ docker-compose run web rails new . --force --database=postgresql --skip-bundle

First, Compose will build the image for the `web` service using the `Dockerfile`. Then it'll run `rails new` inside a new container, using that image. Once it's done, you should have a fresh app generated:

    $ ls
    Dockerfile   app          docker-compose.yml      tmp
    Gemfile      bin          lib          vendor
    Gemfile.lock condocker-compose       log
    README.rdoc  condocker-compose.ru    public
    Rakefile     db           test

Uncomment the line in your new `Gemfile` which loads `therubyracer`, so we've got a Javascript runtime:

    gem 'therubyracer', platforms: :ruby

Now that we've got a new `Gemfile`, we need to build the image again. (This, and changes to the Dockerfile itself, should be the only times you'll need to rebuild).

    $ docker-compose build

The app is now bootable, but we're not quite there yet. By default, Rails expects a database to be running on `localhost` - we need to point it at the `db` container instead. We also need to change the database and username to align with the defaults set by the `postgres` image.

Open up your newly-generated `database.yml`. Replace its contents with the following:

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

We can now boot the app.

    $ docker-compose up

If all's well, you should see some PostgreSQL output, and then—after a few seconds—the familiar refrain:

    myapp_web_1 | [2014-01-17 17:16:29] INFO  WEBrick 1.3.1
    myapp_web_1 | [2014-01-17 17:16:29] INFO  ruby 2.0.0 (2013-11-22) [x86_64-linux-gnu]
    myapp_web_1 | [2014-01-17 17:16:29] INFO  WEBrick::HTTPServer#start: pid=1 port=3000

Finally, we just need to create the database. In another terminal, run:

    $ docker-compose run web rake db:create

And we're rolling—your app should now be running on port 3000 on your docker daemon (if you're using boot2docker, `boot2docker ip` will tell you its address).

![Screenshot of Rails' stock index.html](https://orchardup.com/static/images/docker-compose-rails-screenshot.png)
