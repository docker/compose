---
layout: default
title: Fig | Fast, isolated development environments using Docker
full_width: true
body_class: homepage
---

<div class="leader">
  <div class="container">
    <div class="row">
      <div class="col-md-10 col-md-offset-1">
        <h1>Development environments made effortless.</h1>
        <p>Use Docker to isolate your app’s dependencies. Install and boot in a single command.</p>
      </div>
    </div>
  </div>
</div>

<div class="homepage-section">
  <div class="container">
    <div class="row">
      <div class="col-md-6">
        <h2 class="text-center">Configure</h2>
        <p>Define your services in <code>fig.yml</code>. Everything’s isolated.</p>

        <pre>web:
  build: .
  command: python app.py
  links:
   - db
  ports:
   - "8000:8000"
db:
  image: orchardup/postgresql</pre>

        <p>You can use a <a href="http://docs.docker.io/en/latest/reference/builder/">Dockerfile</a> to set up a service’s environment:</p>

        <pre>FROM orchardup/python:2.7
ADD . /code
WORKDIR /code
RUN pip install -r requirements.txt</pre>

        <p class="text-right"><a href="docs.html">Read the quick start guide &rarr;</a></p>
      </div>

      <div class="col-md-6">
        <h2 class="text-center">Run</h2>
        <p>Type <code>fig up</code>, and Fig will start and run your entire app.</p>
        <img src="https://orchardup.com/static/images/fig-example-large.f96065fc9e22.gif">
      </div>
    </div>
  </div>
</div>

<div class="homepage-section call-to-action">
  <div class="container">
    <div class="text-center">
      <big>Sound like fun? Let’s get started.</big>
      <a href="install.html" class="btn btn-primary btn-lg">Install &rarr;</a>
    </div>
  </div>
</div>

<div class="homepage-section why">
  <div class="container">
    <h2 class="text-center">Why use Fig?</h2>

    <div class="row">
      <div class="col-md-6">
        <h3>Isolation</h3>
        <p>Every service runs in its own Docker container, with its dependencies and nothing else. <strong>No more installing Postgres on your laptop. No more juggling Ruby and Python versions. No more gemsets or virtualenv.</strong></p>

        <h3>Simplicity</h3>
        <p>Your app’s environment is minimally described. <strong>No more configuration management</strong>, bless its baroque little cotton socks.</p>
      </div>
      <div class="col-md-6">
        <h3>Repeatability</h3>
        <p>Anyone with Fig installed can check out your repository, type <code>fig up</code> and be running in moments. <strong>No more spending half a day getting new developers set up.</strong></p>

        <h3>Speed</h3>
        <p>Thanks to lightweight containers and Docker’s layered filesystem model, build and boot times are minimal. <strong>No more 6GB VM images.</strong></p>
      </div>
    </div>
  </div>
</div>
