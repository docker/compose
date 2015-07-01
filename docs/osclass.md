<!--[metadata]>
+++
title = "Getting started with Docker Compose and Osclass"
description = "Migrating and running an Osclass installation."
keywords = ["docker, example, package installation, osclass, compose"]
[menu.main]
parent="smn_workw_compose"
weight=6
+++
<![end-metadata]-->


# Getting started with Docker Compose and Osclass

This document teaches you how to migrate an Osclass installation to a Docker
infrastructure. When you are done, your Docker infrastructure will contain six Docker containers, three  
server containers and three volume containers:

* Mysql database
* Postfix server
* Apache2 Webserver
* Osclass volume 
* Database volume
* Backup volume

This configuration was tested on Osclass 3.5.

## Prerequisites

Before you begin, make sure you have installed the following software:

* VirtualBox
* Docker
* Docker Machine 
* Docker compose 

Refer to the appropriated Docker documentation for different distributions for installing.

## Set up the servers

1. Create the virtualbox machine.

        $ docker-machine create --driver virtualbox dockerenv 

        $ docker-machine active dockerenv

        $ $(docker-machine env dockerenv)

1. Download the scripts using `git clone`.

        $ git clone https://github.com/XaviOutside/MIGRATION_OSCLASS_2_DOCKER.git
        
1. Change to the folder directory.

        $ cd MIGRATION_OSCLASS_2_DOCKER

1. Open the  in `common.env` file in your favorite editor.

        $ vi common.env 


1. Set the `DOMAIN` and `MYSQL_ROOT_PASSWORD` to contain your domain and password.

         DOMAIN=yourdomain.org
         MYSQL_ROOT_PASSWORD=yourpassword

1. Save and close the file.

1. Build containers.

        $ docker-compose build

1. Run containers.

        $ docker-compose up

1. Launch the `http://your_ip_docker_machine` URL in your favorite browses.

    If you don't know it, you can get your `ip` with the following command:

        $ docker-machine ip
     

## Import data

1. Create a `tar.gz` file of your current Osclass server.

1. Replace the `backup_osclass.tar.gz` in the `migration` folder.
     
1. Create a dump file of your Osclass database.

1. Replace the `backup.mysql.sql` in the `migration` folder.
     
1. Copy the `migration` folder into the Osclass container.

        $ tar -cf - migration | docker exec -i <NAME_OSCLASS_CONTAINER>  /bin/tar -C / -xf -

1. Run the `osclass_init.sh` script to import the files and database data into the volume containers.

        $ docker exec -it <NAME_OSCLASS_CONTAINER> bash /osclass_init.sh

## Backups

Generate a backup of our osclass files and database.

    $ docker exec -it <NAME_OSCLASS_CONTAINER> bash /osclass_backup.sh

## Others

There is an `INSTALL.sh` file if you want to launch without `docker-compose`.

## More Compose documentation

- [User guide](/)
- [Installing Compose](install.md)
- [Get started with Django](django.md)
- [Get started with Rails](rails.md)
- [Get started with Wordpress](wordpress.md)
- [Migration: Dockerize osclass](osclass.md)
- [Command line reference](cli.md)
- [Yaml file reference](yml.md)
- [Compose environment variables](env.md)
- [Compose command line completion](completion.md)
