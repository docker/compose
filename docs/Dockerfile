FROM ubuntu:13.10
RUN apt-get -qq update && apt-get install -y ruby1.8 bundler python
RUN locale-gen en_US.UTF-8
ADD Gemfile /code/
ADD Gemfile.lock /code/
WORKDIR /code
RUN bundle install
ADD . /code
EXPOSE 4000
CMD bundle exec jekyll build
