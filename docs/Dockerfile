FROM docs/base:latest
MAINTAINER Mary Anthony <mary@docker.com> (@moxiegirl)

# Get source for the other public-sub projects; private are still not picked up
#
RUN svn checkout https://github.com/docker/tutorials/trunk/docs/mac /docs/content/mac \
&& svn checkout https://github.com/docker/tutorials/trunk/docs/windows /docs/content/windows \
&& svn checkout https://github.com/docker/tutorials/trunk/docs/linux /docs/content/linux

RUN svn checkout https://github.com/docker/docker/trunk/docs /docs/content/ 

RUN svn checkout https://github.com/docker/swarm/trunk/docs /docs/content/swarm  \
&& svn checkout https://github.com/docker/machine/trunk/docs /docs/content/machine \
&& svn checkout https://github.com/docker/distribution/trunk/docs /docs/content/registry \
&& svn checkout https://github.com/kitematic/kitematic/trunk/docs /docs/content/kitematic \
&& svn checkout https://github.com/docker/opensource/trunk/docs /docs/content/opensource 

# Copy local Compose changes to the build
#
COPY . /src
COPY . /docs/content/compose
 
# Run the build ssed over the markdown to convert to Hugo crosss-references
#
RUN curl -sSL https://raw.githubusercontent.com/docker/docs.docker.com/master/touch-up.sh > /src/touch-up.sh \
 && chmod +x /src/touch-up.sh \
 && touch /docs/build.json \
 && /src/touch-up.sh /docs
 