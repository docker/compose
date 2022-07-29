# docker compose restart

<!---MARKER_GEN_START-->
Restart service containers

### Options

| Name | Type | Default | Description |
| --- | --- | --- | --- |
| `-t`, `--timeout` | `int` | `10` | Specify a shutdown timeout in seconds |


<!---MARKER_GEN_END-->

## Description

Restarts all stopped and running services, or the specified services only.

If you make changes to your `compose.yml` configuration, these changes are not reflected
after running this command. For example, changes to environment variables (which are added
after a container is built, but before the container's command is executed) are not updated
after restarting.

If you are looking to configure a service's restart policy, please refer to
[restart](https://github.com/compose-spec/compose-spec/blob/master/spec.md#restart)
or [restart_policy](https://github.com/compose-spec/compose-spec/blob/master/deploy.md#restart_policy).
