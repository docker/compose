# Contributing to the Docker Compose documentation

The documentation in this directory is part of the [https://docs.docker.com](https://docs.docker.com) website.  Docker uses [the Hugo static generator](http://gohugo.io/overview/introduction/) to convert project Markdown files to a static HTML site.

You don't need to be a Hugo expert to contribute to the compose documentation. If you are familiar with Markdown, you can modify the content in the `docs` files.

If you want to add a new file or change the location of the document in the menu, you do need to know a little more.

## Documentation contributing workflow

1. Edit a Markdown file in the tree.

2. Save your changes.

3. Make sure you are in the `docs` subdirectory.

4. Build the documentation.

        $ make docs
         ---> ffcf3f6c4e97
        Removing intermediate container a676414185e8
        Successfully built ffcf3f6c4e97
        docker run --rm -it  -e AWS_S3_BUCKET -e NOCACHE -p 8000:8000 -e DOCKERHOST "docs-base:test-tooling" hugo server --port=8000 --baseUrl=192.168.59.103 --bind=0.0.0.0
        ERROR: 2015/06/13 MenuEntry's .Url is deprecated and will be removed in Hugo 0.15. Use .URL instead.
        0 of 4 drafts rendered
        0 future content
        12 pages created
        0 paginator pages created
        0 tags created
        0 categories created
        in 55 ms
        Serving pages from /docs/public
        Web Server is available at http://0.0.0.0:8000/
        Press Ctrl+C to stop

5. Open the available server in your browser.

    The documentation server has the complete menu but only the Docker Compose
    documentation resolves.  You can't access the other project docs from this
    localized build.

## Tips on Hugo metadata and menu positioning

The top of each Docker Compose documentation file contains TOML metadata. The metadata is commented out to prevent it from appearing in GitHub.

    <!--[metadata]>
    +++
    title = "Extending services in Compose"
    description = "How to use Docker Compose's extends keyword to share configuration between files and projects"
    keywords = ["fig, composition, compose, docker, orchestration, documentation, docs"]
    [menu.main]
    parent="smn_workw_compose"
    weight=2
    +++
    <![end-metadata]-->

The metadata alone has this structure:

    +++
    title = "Extending services in Compose"
    description = "How to use Docker Compose's extends keyword to share configuration between files and projects"
    keywords = ["fig, composition, compose, docker, orchestration, documentation, docs"]
    [menu.main]
    parent="smn_workw_compose"
    weight=2
    +++

The `[menu.main]` section refers to navigation defined [in the main Docker menu](https://github.com/docker/docs-base/blob/hugo/config.toml). This metadata says *add a menu item called* Extending services in Compose *to the menu with the* `smn_workdw_compose` *identifier*.  If you locate the menu in the configuration, you'll find *Create multi-container applications* is the menu title.

You can move an article in the tree by specifying a new parent. You can shift the location of the item by changing its weight.  Higher numbers are heavier and shift the item to the bottom of menu. Low or no numbers shift it up.


## Other key documentation repositories

The `docker/docs-base` repository contains [the Hugo theme and menu configuration](https://github.com/docker/docs-base). If you open the `Dockerfile` you'll see the `make docs` relies on this as a base image for building the Compose documentation.

The `docker/docs.docker.com` repository contains [build system for building the Docker documentation site](https://github.com/docker/docs.docker.com). Fork this repository to build the entire documentation site.
