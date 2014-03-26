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
        <h1>Fast, isolated development<br>environments using Docker.</h1>
        <p>Fig makes setting up your environment ridiculously simple, and boots your app with a single command.</p>
      </div>
    </div>
  </div>
</div>

<div class="homepage-section">
  <div class="container">
    <div class="row">
      <div class="col-md-6">
        <h2 class="text-center">Configure</h2>
        <p>Define your app’s services in <code>fig.yml</code>. Everything’s isolated.</p>

        <pre>web:
  build: .
  command: python app.py
  links:
   - db
  ports:
   - "8000:8000"
db:
  image: orchardup/postgresql</pre>

        <p>You can use a <a href="">Dockerfile</a> to install a service’s dependencies:</p>

        <pre>FROM orchardup/python:2.7
ADD . /code
WORKDIR /code
RUN pip install -r requirements.txt</pre>

        <p class="text-right"><a href="quickstart.html">Read the quickstart guide &rarr;</a></p>
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

<div class="homepage-section">
  <div class="container">
    <h2 class="text-center">Why use Fig?</h2>

    <div class="row">
      <div class="col-md-6">
        <h3>Isolation</h3>
        <p>Not just your app: every invididual service runs in its own Docker container, with its dependencies and nothing else. No more installing Postgres on your laptop; no more juggling Ruby and Python versions.</p>

        <h3>Simplicity</h3>
        <p>Your app’s environment is minimally described. No more configuration management, bless its baroque little cotton socks.</p>
      </div>
      <div class="col-md-6">
        <h3>Repeatability</h3>
        <p>Anyone with Fig installed can check out your repository, type <code>fig up</code> and be running in moments. No more spending half a day getting set up on a new project.</p>

        <h3>Speed</h3>
        <p>Thanks to lightweight containers and Docker’s layered filesystem model, build and boot times are minimal. No more 6GB VM images.</p>
      </div>
    </div>
  </div>
</div>
