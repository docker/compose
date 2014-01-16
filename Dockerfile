FROM stackbrew/ubuntu:12.04
RUN apt-get update -qq
RUN apt-get install -y python python-pip
ADD requirements.txt /code/
WORKDIR /code/
RUN pip install -r requirements.txt
ADD requirements-dev.txt /code/
RUN pip install -r requirements-dev.txt
ADD . /code/
