
## Description

This is the equivalent of `docker exec` targeting a Compose service. 

With this subcommand you can run arbitrary commands in your services. Commands are by default allocating a TTY, so 
you can use a command such as `docker compose exec web sh` to get an interactive prompt.
