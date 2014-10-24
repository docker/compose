---
layout: default
title: Getting started with Fig and Wordpress
---

Getting started with Fig and Wordpress
======================================

Fig makes it nice and easy to run Wordpress in an isolated environment. [Install Fig](install.html), then download Wordpress into the current directory:

    $ curl https://wordpress.org/latest.tar.gz | tar -xvzf -

This will create a directory called `wordpress`, which you can rename to the name of your project if you wish. Inside that directory, we create `Dockerfile`, a file that defines what environment your app is going to run in:

```
FROM orchardup/php5
ADD . /code
```

This instructs Docker on how to build an image that contains PHP and Wordpress. For more information on how to write Dockerfiles, see the [Docker user guide](https://docs.docker.com/userguide/dockerimages/#building-an-image-from-a-dockerfile) and the [Dockerfile reference](http://docs.docker.com/reference/builder/).

Next up, `fig.yml` starts our web service and a separate MySQL instance:

```
web:
  build: .
  command: php -S 0.0.0.0:8000 -t /code
  ports:
    - "8000:8000"
  links:
    - db
  volumes:
    - .:/code
db:
  image: orchardup/mysql
  environment:
    MYSQL_DATABASE: wordpress
```

Two supporting files are needed to get this working - first up, `wp-config.php` is the standard Wordpress config file with a single change to point the database configuration at the `db` container:

```
<?php
define('DB_NAME', 'wordpress');
define('DB_USER', 'root');
define('DB_PASSWORD', '');
define('DB_HOST', "db:3306");
define('DB_CHARSET', 'utf8');
define('DB_COLLATE', '');

define('AUTH_KEY',         'put your unique phrase here');
define('SECURE_AUTH_KEY',  'put your unique phrase here');
define('LOGGED_IN_KEY',    'put your unique phrase here');
define('NONCE_KEY',        'put your unique phrase here');
define('AUTH_SALT',        'put your unique phrase here');
define('SECURE_AUTH_SALT', 'put your unique phrase here');
define('LOGGED_IN_SALT',   'put your unique phrase here');
define('NONCE_SALT',       'put your unique phrase here');

$table_prefix  = 'wp_';
define('WPLANG', '');
define('WP_DEBUG', false);

if ( !defined('ABSPATH') )
    define('ABSPATH', dirname(__FILE__) . '/');

require_once(ABSPATH . 'wp-settings.php');
```

Finally, `router.php` tells PHP's built-in web server how to run Wordpress:

```
<?php

$root = $_SERVER['DOCUMENT_ROOT'];
chdir($root);
$path = '/'.ltrim(parse_url($_SERVER['REQUEST_URI'])['path'],'/');
set_include_path(get_include_path().':'.__DIR__);
if(file_exists($root.$path))
{
    if(is_dir($root.$path) && substr($path,strlen($path) - 1, 1) !== '/')
        $path = rtrim($path,'/').'/index.php';
    if(strpos($path,'.php') === false) return false;
    else {
        chdir(dirname($root.$path));
        require_once $root.$path;
    }
}else include_once 'index.php';
```

With those four files in place, run `fig up` inside your Wordpress directory and it'll pull and build the images we need, and then start the web and database containers. You'll then be able to visit Wordpress at port 8000 on your docker daemon (if you're using boot2docker, `boot2docker ip` will tell you its address).
