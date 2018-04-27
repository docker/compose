FROM python:3.6
RUN mkdir -p /src && pip install -U Jinja2==2.10 \
    PyGithub==1.39 \
    pypandoc==1.4 \
    GitPython==2.1.9 \
    requests==2.18.4 \
    twine==1.11.0 && \
    apt-get update && apt-get install -y pandoc

VOLUME /src/script/release
WORKDIR /src
COPY . /src
RUN python setup.py develop
ENTRYPOINT ["python", "script/release/release.py"]
CMD ["--help"]
