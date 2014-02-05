---
layout: default
title: Getting started with Fig and Wordpress
---

Getting started with Fig and Wordpress
======================================

Fig makes it nice and easy to run Wordpress in an isolated environment. [Install Fig](install.html), then write a `Dockerfile` which installs PHP and Wordpress:

```
FROM orchardup/php5

ADD http://wordpress.org/wordpress-3.8.1.tar.gz /wordpress.tar.gz
RUN mv /wordpress-3.8.1/wordpress /wordpress
ADD wp-config.php /wordpress/wp-config.php

ADD router.php /router.php
```

This instructs Docker on how to build an image that contains PHP and Wordpress. For more information on how to write Dockerfiles, see the [Dockerfile tutorial](https://www.docker.io/learn/dockerfile/) and the [Dockerfile reference](http://docs.docker.io/en/latest/use/builder/).

Next up, `fig.yml` starts our web service and a separate MySQL instance:

```
web:
  build: .
  command: php -S 0.0.0.0:8000 -t /wordpress
  ports:
    - 8000:8000
  links:
    - db
db:
  image: orchardup/mysql
  ports:
    - 3306:3306
  environment:
    MYSQL_DATABASE: wordpress
```

Our Dockerfile relies on two supporting files - first up, `wp-config.php` is the standard Wordpress config file with a single change to make it read the MySQL host and port from the environment variables passed in by Fig:

```
<?php
define('DB_NAME', 'wordpress');
define('DB_USER', 'root');
define('DB_PASSWORD', '');
define('DB_HOST', getenv("DB_1_PORT_3306_TCP_ADDR") . ":" . getenv("DB_1_PORT_3306_TCP_PORT"));
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

With those four files in place, run `fig up` and it'll pull and build the images we need, and then start the web and database containers. You'll then be able to visit Wordpress and set it up by visiting [localhost:8000](http://localhost:8000) - or [localdocker:8000](http://localdocker:8000) if you're using docker-osx.
