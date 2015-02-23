FROM docs/base:latest
MAINTAINER Sven Dowideit <SvenDowideit@docker.com> (@SvenDowideit)

# to get the git info for this repo
COPY . /src

# Reset the /docs dir so we can replace the theme meta with the new repo's git info
RUN git reset --hard

RUN grep "__version" /src/compose/__init__.py | sed "s/.*'\(.*\)'/\1/" > /docs/VERSION
COPY docs/* /docs/sources/compose/
COPY docs/mkdocs.yml /docs/mkdocs-compose.yml

# Then build everything together, ready for mkdocs
RUN /docs/build.sh
