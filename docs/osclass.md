<!--[metadata]>
+++
title = "Migrate Osclass installation to Docker Compose"
description = "Migrating and running an Osclass installation."
keywords = ["docker, example, package installation, osclass, compose"]
[menu.main]
parent="smn_workw_compose"
weight=7
+++
<![end-metadata]-->


# Migrate Osclass installation to Docker Compose

This document teaches you how to migrate an Osclass installation to a Docker
infrastructure. When you are done, your Docker infrastructure will contain six
Docker containers, three server containers, and three volume containers:

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
* [Docker](https://docs.docker.com/installation/#installation)
* [Docker Machine](http://docs.docker.com/machine/install-machine/)
* [Docker Compose](install.md)

Refer to the appropriate Docker documentation for different distributions for installing.

## Provision the servers with machine

XAVIER: Add an intro sentence or two about how this procedure users yoru scripts and what they are doing

1. Create the VirtualBox machine.

        $ docker-machine create --driver virtualbox dockerenv 

        $ docker-machine active dockerenv

        $ $(docker-machine env dockerenv)

2. Download the scripts using `git clone`.

        $ git clone https://github.com/XaviOutside/MIGRATION_OSCLASS_2_DOCKER.git
        
3. Change to the folder directory.

        $ cd MIGRATION_OSCLASS_2_DOCKER

4. Open the `common.env` file in your favorite editor, for example:

        $ vi common.env 

5. Set the `DOMAIN` and `MYSQL_ROOT_PASSWORD` to contain your domain and password.

         DOMAIN=yourdomain.org
         MYSQL_ROOT_PASSWORD=yourpassword

6. Save and close the file.

7. Build containers.

        $ docker-compose build

8. Run containers.

        $ docker-compose up

9. Launch the `http://your_ip_docker_machine` URL in your favorite browses.

   Use the following command to get your `ip` with machine:
   
        $ docker-machine ip
     

You can also use the `INSTALL.sh` file if you want to launch without `docker-compose`.


## Backup your Osclass server and import the data

1. Backup to a `tar.gz` file of your current Osclass server.

2. Replace the `backup_osclass.tar.gz` in the `migration` folder.
     
3. Create a `backup.mysql.sql` dump file of your Osclass database.

4. Replace the `backup.mysql.sql` in the `migration` folder.
     
5. Copy the `migration` folder into the Osclass container.

        $ tar -cf - migration | docker exec -i <NAME_OSCLASS_CONTAINER>  /bin/tar -C / -xf -

6. Run the `osclass_init.sh` script to import the files and database data into the volume containers.

        $ docker exec -it <NAME_OSCLASS_CONTAINER> bash /osclass_init.sh

7. Generate a backup of the Osclass files and database.

        $ docker exec -it <NAME_OSCLASS_CONTAINER> bash /osclass_backup.sh

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
