FROM docs/base:latest
MAINTAINER Mary Anthony <mary@docker.com> (@moxiegirl)

# To get the git info for this repo
COPY . /src

COPY . /docs/content/compose/

RUN svn checkout https://github.com/docker/docker/trunk/docs /docs/content/docker
RUN svn checkout https://github.com/docker/swarm/trunk/docs /docs/content/swarm
RUN svn checkout https://github.com/docker/machine/trunk/docs /docs/content/machine
RUN svn checkout https://github.com/docker/distribution/trunk/docs /docs/content/registry
RUN svn checkout https://github.com/docker/tutorials/trunk/docs /docs/content/tutorials
RUN svn checkout https://github.com/docker/opensource/trunk/docs /docs/content


# Sed to process GitHub Markdown
# 1-2 Remove comment code from metadata block
# 3 Change ](/word to ](/project/ in links
# 4 Change ](word.md) to ](/project/word)
# 5 Remove .md extension from link text
# 6 Change ](../ to ](/project/word) 
# 7 Change ](../../ to ](/project/ --> not implemented
# 
# 
RUN /src/pre-process.sh /docs
