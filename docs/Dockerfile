FROM docs/base:latest
MAINTAINER Mary Anthony <mary@docker.com> (@moxiegirl)

RUN svn checkout https://github.com/docker/docker/trunk/docs /docs/content/engine
RUN svn checkout https://github.com/docker/swarm/trunk/docs /docs/content/swarm
RUN svn checkout https://github.com/docker/machine/trunk/docs /docs/content/machine
RUN svn checkout https://github.com/docker/distribution/trunk/docs /docs/content/registry
RUN svn checkout https://github.com/kitematic/kitematic/trunk/docs /docs/content/kitematic
RUN svn checkout https://github.com/docker/tutorials/trunk/docs /docs/content/tutorials
RUN svn checkout https://github.com/docker/opensource/trunk/docs /docs/content

ENV PROJECT=compose
# To get the git info for this repo
COPY . /src

COPY . /docs/content/$PROJECT/
