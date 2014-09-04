FROM debian:wheezy
RUN apt-get update -qq && apt-get install -qy python python-pip python-dev && apt-get clean
ADD requirements.txt /code/
WORKDIR /code/
RUN pip install -r requirements.txt
ADD requirements-dev.txt /code/
RUN pip install -r requirements-dev.txt
ADD . /code/
RUN python setup.py develop
RUN useradd -d /home/user -m -s /bin/bash user
RUN chown -R user /code/
USER user
