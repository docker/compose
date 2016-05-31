from __future__ import absolute_import
from __future__ import unicode_literals

import functools
import logging
import operator

from compose.common import const
from compose.common import signals
from compose.common import utils
from compose.config import config
from compose.config import environment
from compose.core import dockerclient as dc
from compose.core import project
from compose.core import service as compose_service

if not const.IS_WINDOWS_PLATFORM:
    from dockerpty import pty as dockerpty

LOG = logging.getLogger(__name__)
TCP = 'tcp'
UDP = 'udp'


def setup_exec_on_container(action):

    @functools.wraps(action)
    def wraps(*args, **kwargs):
        (self, service_name, exec_command,
         if_preveleged, username, positional_args) = args
        tty, container_index = (kwargs.get('tty', True),
                                kwargs.get('container_index', 1))

        service = self.get_service(service_name)

        create_exec_options = {
            "privileged": if_preveleged,
            "user": username,
            "tty": tty,
            "stdin": tty,
        }
        command = [exec_command, ] + positional_args
        container = service.get_container(number=container_index)
        exec_id = container.create_exec(command, **create_exec_options)
        return action(self, container, exec_id, tty)

    return wraps


def validate_run(action):
    @functools.wraps(action)
    def wraps(*args, **kwargs):
        if const.IS_WINDOWS_PLATFORM and not kwargs.get('detached_mode', False):
            raise Exception("Interactive mode is not yet supported on Windows.")

        if kwargs.get('publish_ports') and kwargs.get('service-ports'):
            raise Exception(
                'Service port mapping and manual port mapping '
                'can not be used together.')

        return action(*args, **kwargs)

    return wraps


class ComposeAPI(environment.Environment, project.Project):

    def __init__(self, docker_daemon_host,
                 docker_daemon_port,
                 docker_daemon_api_version,
                 docker_daemon_connection_timeout,
                 docker_daemon_tls_cert=None, verify_tls=None, ca_cert=None,
                 assert_hostname=None, assert_fingerprint=None, logger=None):
        """
        Docker Compose API
        :param docker_daemon_host: Docker host
        :param docker_daemon_port: Docker port
        :param docker_daemon_api_version: Docker HTTP API version
        :param docker_daemon_connection_timeout: Docker HTTP connection timeout
        :param docker_daemon_tls_cert: Docker TLS certificate
        :param verify_tls: Enable/Disable TLS verification
        :param ca_cert: CA certificate
        :param assert_hostname: Enable/Disable hostname verification
        :param assert_fingerprint: Enable/Disable certificate fingerprint
        """
        self.host = docker_daemon_host
        self.port = docker_daemon_port
        self.api_version = docker_daemon_api_version
        self.connection_timeout = docker_daemon_connection_timeout
        self.tls_config = dc.tls.TLSConfig(
            client_cert=docker_daemon_tls_cert,
            verify=verify_tls,
            ca_cert=ca_cert,
            assert_hostname=assert_hostname,
            assert_fingerprint=assert_fingerprint
        )
        self.client = dc.client.Client(**{
            'base_url': "%s:%s" % (self.host, self.port),
            'tls_config': self.tls_config,
            'version': self.api_version,
            'timeout': self.connection_timeout,
        })
        self.environment = super(ComposeAPI, self).init_from_env()
        self.log = logger if logger else LOG

    def get_project(self, project_name, compose_file):
        """
        Defines compose project for further processing
        :param project_name: Non-existing compose project name
        :param compose_file: compose file path
        :return:
        """
        self.compose_details = config.ConfigDetails(
            None, [compose_file], environment=self.environment)
        self.config_data = config.load(self.compose_details)
        return self.from_config(project_name, self.config_data, self.client)

    def build(self, service_names=None, no_cache=False, pull=False, force_rm=False):
        """
        Builds a project for given compose config file
        :param service_names: Services from compose config file
        :param no_cache:
        :param pull:
        :param force_rm:
        :return:
        """
        return super(ComposeAPI, self).build(service_names=service_names,
                                             no_cache=no_cache,
                                             pull=pull,
                                             force_rm=force_rm)

    @property
    def config(self):
        return self.config_data

    def create(self, service_names=None, start_deps=None,
               abort_on_container_exit=False,
               force_recreate=False, build=None, no_build=None,
               remove_orphaned=True, detached_mode=None,
               timeout=const.DEFAULT_TIMEOUT,
               no_recreate=None,):
        with utils.up_shutdown_context(
                self, service_names, timeout, detached_mode, is_cli_call=False):
            return super(ComposeAPI, self).up(
                service_names=service_names,
                start_deps=start_deps,
                strategy=utils.convergence_strategy_from_opts(
                    no_recreate=no_recreate,
                    force_recreate=force_recreate,
                    force_recreate_strategy=project.ConvergenceStrategy.always,
                    no_recreate_strategy=project.ConvergenceStrategy.never,
                    alternative_strategy=project.ConvergenceStrategy.changed),
                do_build=utils.build_action_from_opts(
                    build=build, no_build=no_build,
                    build_action=project.BuildAction.force,
                    no_build_action=project.BuildAction.skip,
                    alternative_action=project.BuildAction.none,
                ),
                timeout=timeout,
                detached=detached_mode,
                remove_orphans=remove_orphaned)

    def down(self, remove_images=None, include_volumes=None, remove_orphans=False):
        """

        :param remove_images:
        :param include_volumes:
        :param remove_orphans:
        :return:
        """
        image_type = utils.image_type_from_opt(
            'remove_images', remove_images, image_type_class=compose_service.ImageType)
        return super(ComposeAPI, self).down(image_type, include_volumes,
                                            remove_orphans=remove_orphans)

    def events(self, service_names=None):
        """

        :param service_names:
        :return:
        """
        return super(ComposeAPI, self).events(service_names=service_names)

    @setup_exec_on_container
    def async_exec(self, container, exec_id, tty):
        """
        Runs asynchronous exec command at specific container identified
        by a service and container index
        :param service_name: Service name required to identify container.
        :param exec_command:
        :param if_preveleged: Give extended privileges to the process.
        :param username: Run the command as this user.
        :param positional_args: List of args for exec command.
        :param tty: Disable pseudo-tty allocation. By default Compose API allocates a TTY.
        :param detached_mode: Run command in the background.
        :param container_index: Index of the container if there are multiple
        :return:
        """
        if const.IS_WINDOWS_PLATFORM:
            raise Exception(
                "Interactive mode is not yet supported on Windows."
            )
        return container.start_exec(exec_id, tty=tty)

    @setup_exec_on_container
    def sync_exec(self, container, exec_id, tty):
        """
        Runs synchronous exec command at specific container identified
        by a service and container index
        :param service_name: Service name required to identify container.
        :param exec_command:
        :param if_preveleged: Give extended privileges to the process.
        :param username: Run the command as this user.
        :param positional_args: List of args for exec command.
        :param tty: Disable pseudo-tty allocation. By default Compose API allocates a TTY.
        :param detached_mode: Run command in the background.
        :param container_index: Index of the container if there are multiple
        :return:
        """
        signals.set_signal_handler_to_shutdown()
        try:
            operation = dockerpty.ExecOperation(
                self.client,
                exec_id,
                interactive=tty,
            )
            pty = dockerpty.PseudoTerminal(self.client, operation)
            pty.start()
        except signals.ShutdownException:
            self.log.info("received shutdown exception: closing.")

        return self.client.exec_inspect(exec_id).get("ExitCode")

    def kill_container(self, service_names=None, **options):
        """
        Force kill stopped containers
        :param service_names:
        :param options:
        :return:
        """
        return super(ComposeAPI, self).kill(service_names=service_names, **options)

    def pause_container(self, service_names=None, **options):
        """
        Pause services
        :param service_names:
        :param options:
        :return:
        """
        return super(ComposeAPI, self).pause(service_names=service_names, **options)

    def get_container_port(self, service_names, container_index=1,
                           protocol=TCP, private_port=False):
        """
        Get public port for a port binding.
        :param service_names:
        :param container_index:
        :param protocol:
        :param private_port:
        :return:
        """
        service = self.get_service(service_names)
        container = service.get_container(number=container_index)
        return container.get_local_port(private_port, protocol=protocol)

    def list_containers(self, service_names=None):
        """
        List containers
        :param service_names:
        :return:
        """
        return sorted(
            self.containers(service_names=service_names, stopped=True) +
            self.containers(service_names=service_names),
            key=operator.attrgetter('name'))

    def pull_image(self, service_names=None, ignore_pull_failures=False):
        """
        Pulls images for services
        :param service_names:
        :param ignore_pull_failures:
        :return:
        """
        return super(ComposeAPI, self).pull(service_names=service_names,
                                            ignore_pull_failures=ignore_pull_failures)

    def remove_service_containers(self, service_names=None,
                                  remove_anonymous_attached_volumes=False):
        """
        Removes stopped service containers
        :param service_names:
        :param remove_anonymous_attached_volumes:
        :return:
        """
        return self.remove_stopped(service_names=service_names,
                                   v=remove_anonymous_attached_volumes,
                                   one_off=project.OneOffFilter.include)

    @validate_run
    def run(self, name, service_name, command=None,
            command_args=None, entry_point=None,
            os_environment=None, run_as_user_or_uid=None,
            no_deps=False, remove_after_run=False,
            publish_ports=None, service_ports=None, tty=True,
            detached_mode=True, container_working_dir=None):
        """
        Run a one-off command on a service.
        :param name: Assign a name to the container.
        :type name: str
        :param service_name: Service name from compose config
        :type service_name: str
        :param command: Shell command to execute over container.
        :type command: str
        :param command_args: Shell command opts
        :type command_args: list
        :param entry_point: Override the entry point of the image.
        :type entry_point: str
        :param os_environment: Set an environment variable
        :type os_environment: dict
        :param run_as_user_or_uid: Run as specified username or UID.
        :type run_as_user_or_uid: str
        :param no_deps: Don't start linked services.
        :type no_deps: bool
        :param remove_after_run: Remove container after run. Ignored in detached mode.
        :type remove_after_run: bool
        :param publish_ports: Publish a container's port(s) to the host
        :type publish_ports: list
        :param service_ports: Run command with the service's ports enabled and mapped to the host.
        :type service_ports: list
        :param tty: Disable pseudo-tty allocation. By default TTY is allocated.
        :type tty: bool
        :param detached_mode:
        :type detached_mode: bool
        :param container_working_dir: Working directory inside the container.
        :type container_working_dir: str
        :return: True/False based on the type of execution (async, sync)
        :rtype: bool
        """

        service = self.get_service(service_name)
        if command:
            command = [command, ] + command_args
        else:
            command = service.options.get('command')

        c_opts = utils.prepare_container_opts(
            command, detached_mode, os_environment, entry_point,
            remove_after_run, run_as_user_or_uid, name,
            container_working_dir, service_ports, publish_ports)
        return utils.run_one_off(self, c_opts, service, no_deps,
                                 detached_mode, remove_after_run,
                                 tty, dockerpty, project.ConvergenceStrategy.never)

    def scale_service(self, scale_dict):
        """
        Scales given service names by given scale rate
        :param scale_dict: A dict of service names and scale rate with timeout
        :return:
        """
        for service_name, scale_opts in scale_dict.items():
            self.get_service(service_name).scale(
                scale_opts.get('rate'), scale_opts.get(
                    'timeout', const.DEFAULT_TIMEOUT))

    def start_containers(self, service_names=None,
                         timeout=const.DEFAULT_TIMEOUT, **options):
        """
        Start existing containers
        :param service_names:
        :param options:
        :param timeout
        :return:
        """
        return super(ComposeAPI, self).start(
            service_names=service_names, timeout=timeout, **options)

    def stop_containers(self, service_names=None,
                        timeout=const.DEFAULT_TIMEOUT, **options):
        """
        Stop running containers without removing them.
        :param service_names:
        :param options:
        :param timeout:
        :return:
        """
        return super(ComposeAPI, self).stop(
            service_names=service_names, timeout=timeout, **options)

    def restart_containers(self, service_names=None, timeout=const.DEFAULT_TIMEOUT, **options):
        """
        Restart running containers.
        :param service_names:
        :param options:
        :param timeout:
        :return:
        """
        return super(ComposeAPI, self).restart(
            service_names=service_names, timeout=timeout, **options)

    def unpause_services(self, service_names=None, **options):
        """
        Unpause services.
        :param service_names:
        :param options:
        :return:
        """
        return super(ComposeAPI, self).unpause(service_names=service_names, **options)

    def up_project(self, service_names=None, start_deps=None,
                   abort_on_container_exit=False,
                   force_recreate=False, build=None, no_build=None,
                   remove_orphaned=True, detached_mode=None,
                   timeout=const.DEFAULT_TIMEOUT,
                   no_recreate=None,):
        """
        Builds, (re)creates, starts, and attaches to containers for a service.
        Unless they are already running, this command also starts any linked services.
        :param service_names:
        :param start_deps:
        :param abort_on_container_exit:
        :param force_recreate:
        :param build:
        :param no_build:
        :param remove_orphaned:
        :param detached_mode:
        :param timeout:
        :param no_recreate:
        :return:
        """
        if abort_on_container_exit and detached_mode:
            raise Exception("abort-on_container_exit and "
                            "detached_mode cannot be combined.")

        with utils.up_shutdown_context(
                self, service_names, timeout, detached_mode, is_cli_call=False):
            return super(ComposeAPI, self).up(
                service_names=service_names,
                start_deps=start_deps,
                strategy=utils.convergence_strategy_from_opts(
                    no_recreate=no_recreate,
                    force_recreate=force_recreate,
                    force_recreate_strategy=project.ConvergenceStrategy.always,
                    no_recreate_strategy=project.ConvergenceStrategy.never,
                    alternative_strategy=project.ConvergenceStrategy.changed),
                do_build=utils.build_action_from_opts(
                    build=build, no_build=no_build,
                    build_action=project.BuildAction.force,
                    no_build_action=project.BuildAction.skip,
                    alternative_action=project.BuildAction.none,
                ),
                timeout=timeout,
                detached=detached_mode,
                remove_orphans=remove_orphaned)
