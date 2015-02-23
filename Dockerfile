FROM debian:wheezy

RUN apt-get update -qq

# Compose dependencies
RUN apt-get install -qqy python python-pip python-dev git

# Test dependencies
RUN apt-get install -qqy apt-transport-https ca-certificates curl lxc iptables
RUN curl https://get.docker.com/builds/Linux/x86_64/docker-1.3.3 > /usr/local/bin/docker-1.3.3 && chmod +x /usr/local/bin/docker-1.3.3
RUN curl https://get.docker.com/builds/Linux/x86_64/docker-1.4.1 > /usr/local/bin/docker-1.4.1 && chmod +x /usr/local/bin/docker-1.4.1
RUN curl https://get.docker.com/builds/Linux/x86_64/docker-1.5.0 > /usr/local/bin/docker-1.5.0 && chmod +x /usr/local/bin/docker-1.5.0

RUN apt-get clean

RUN useradd -d /home/user -m -s /bin/bash user
WORKDIR /code/

ADD requirements.txt /code/
RUN pip install -r requirements.txt

ADD requirements-dev.txt /code/
RUN pip install -r requirements-dev.txt

ADD . /code/
RUN python setup.py install

RUN chown -R user /code/

ENTRYPOINT ["/usr/local/bin/docker-compose"]
