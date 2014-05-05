FROM orchardup/python:2.7
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
ENTRYPOINT /usr/local/bin/fig
