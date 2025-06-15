import contextlib
import functools
import json
import logging
import shlex
import re
import subprocess
import sys
from distutils.spawn import find_executable
from inspect import getdoc
from operator import attrgetter

import docker.errors
import docker.utils

from . import errors
from . import signals
from .. import __version__
from ..config import ConfigurationError
from ..config import parse_environment
from ..config import parse_labels
from ..config import resolve_build_args
from ..config.environment import Environment
from ..config.serialize import serialize_config
from ..config.types import VolumeSpec
from ..const import IS_LINUX_PLATFORM
from ..const import IS_WINDOWS_PLATFORM
from ..errors import StreamParseError
from ..metrics.decorator import metrics
from ..parallel import ParallelStreamWriter
from ..progress_stream import StreamOutputError
from ..project import get_image_digests
from ..project import MissingDigests
from ..project import NoSuchService
from ..project import OneOffFilter
from ..project import ProjectError
from ..service import BuildAction
from ..service import BuildError
from ..service import ConvergenceStrategy
from ..service import ImageType
from ..service import NeedsBuildError
from ..service import OperationFailedError
from ..utils import filter_attached_for_up
from .colors import AnsiMode
from .command import get_config_from_options
from .command import get_project_dir
from .command import project_from_options
from .docopt_command import DocoptDispatcher
from .docopt_command import get_handler
from .docopt_command import NoSuchCommand
from .errors import UserError
from .formatter import ConsoleWarningFormatter
from .formatter import Formatter
from .log_printer import build_log_presenters
from .log_printer import LogPrinter
from .utils import get_version_info
from .utils import human_readable_file_size
from .utils import yesno
from compose.metrics.client import MetricsCommand
from compose.metrics.client import Status


if not IS_WINDOWS_PLATFORM:
    from dockerpty.pty import PseudoTerminal, RunOperation, ExecOperation

log = logging.getLogger(__name__)


def main():  # noqa: C901
    signals.ignore_sigpipe()
    command = None
    try:
        _, opts, command = DocoptDispatcher.get_command_and_options(
            TopLevelCommand,
            get_filtered_args(sys.argv[1:]),
            {'options_first': True, 'version': get_version_info('compose')})
    except Exception:
        pass
    try:
        command_func = dispatch()
        command_func()
        if not IS_LINUX_PLATFORM and command == 'help':
            print("\nDocker Compose is now in the Docker CLI, try `docker compose` help")
    except (KeyboardInterrupt, signals.ShutdownException):
        exit_with_metrics(command, "Aborting.", status=Status.CANCELED)
    except (UserError, NoSuchService, ConfigurationError,
            ProjectError, OperationFailedError) as e:
        exit_with_metrics(command, e.msg, status=Status.FAILURE)
    except BuildError as e:
        reason = ""
        if e.reason:
            reason = " : " + e.reason
        exit_with_metrics(command,
                          "Service '{}' failed to build{}".format(e.service.name, reason),
                          status=Status.FAILURE)
    except StreamOutputError as e:
        exit_with_metrics(command, e, status=Status.FAILURE)
    except NeedsBuildError as e:
        exit_with_metrics(command,
                          "Service '{}' needs to be built, but --no-build was passed.".format(
                              e.service.name), status=Status.FAILURE)
    except NoSuchCommand as e:
        commands = "\n".join(parse_doc_section("commands:", getdoc(e.supercommand)))
        if not IS_LINUX_PLATFORM:
            commands += "\n\nDocker Compose is now in the Docker CLI, try `docker compose`"
        exit_with_metrics("", log_msg="No such command: {}\n\n{}".format(
            e.command, commands), status=Status.FAILURE)
    except (errors.ConnectionError, StreamParseError):
        exit_with_metrics(command, status=Status.FAILURE)
    except SystemExit as e:
        status = Status.SUCCESS
        if len(sys.argv) > 1 and '--help' not in sys.argv:
            status = Status.FAILURE

        if command and len(sys.argv) >= 3 and sys.argv[2] == '--help':
            command = '--help ' + command

        if not command and len(sys.argv) >= 2 and sys.argv[1] == '--help':
            command = '--help'

        msg = e.args[0] if len(e.args) else ""
        code = 0
        if isinstance(e.code, int):
            code = e.code

        if not IS_LINUX_PLATFORM and not command:
            msg += "\n\nDocker Compose is now in the Docker CLI, try `docker compose`"

        exit_with_metrics(command, log_msg=msg, status=status,
                          exit_code=code)


def get_filtered_args(args):
    if args[0] in ('-h', '--help'):
        return []
    if args[0] == '--version':
        return ['version']


def exit_with_metrics(command, log_msg=None, status=Status.SUCCESS, exit_code=1):
    if log_msg and command != 'exec':
        if not exit_code:
            log.info(log_msg)
        else:
            log.error(log_msg)

    MetricsCommand(command, status=status).send_metrics()
    sys.exit(exit_code)


def dispatch():
    console_stream = sys.stderr
    console_handler = logging.StreamHandler(console_stream)
    setup_logging(console_handler)
    dispatcher = DocoptDispatcher(
        TopLevelCommand,
        {'options_first': True, 'version': get_version_info('compose')})

    options, handler, command_options = dispatcher.parse(sys.argv[1:])

    ansi_mode = AnsiMode.AUTO
    try:
        if options.get("--ansi"):
            ansi_mode = AnsiMode(options.get("--ansi"))
    except ValueError:
        raise UserError(
            'Invalid value for --ansi: {}. Expected one of {}.'.format(
                options.get("--ansi"),
                ', '.join(m.value for m in AnsiMode)
            )
        )
    if options.get("--no-ansi"):
        if options.get("--ansi"):
            raise UserError("--no-ansi and --ansi cannot be combined.")
        log.warning('--no-ansi option is deprecated and will be removed in future versions. '
                    'Use `--ansi never` instead.')
        ansi_mode = AnsiMode.NEVER

    setup_console_handler(console_handler,
                          options.get('--verbose'),
                          ansi_mode.use_ansi_codes(console_handler.stream),
                          options.get("--log-level"))
    setup_parallel_logger(ansi_mode)
    if ansi_mode is AnsiMode.NEVER:
        command_options['--no-color'] = True
    return functools.partial(perform_command, options, handler, command_options)


def perform_command(options, handler, command_options):
    if options['COMMAND'] in ('help', 'version'):
        # Skip looking up the compose file.
        handler(command_options)
        return

    if options['COMMAND'] == 'config':
        command = TopLevelCommand(None, options=options)
        handler(command, command_options)
        return

    project = project_from_options('.', options)
    command = TopLevelCommand(project, options=options)
    with errors.handle_connection_errors(project.client):
        handler(command, command_options)


def setup_logging(console_handler):
    root_logger = logging.getLogger()
    root_logger.addHandler(console_handler)
    root_logger.setLevel(logging.DEBUG)

    # Disable requests and docker-py logging
    logging.getLogger("urllib3").propagate = False
    logging.getLogger("requests").propagate = False
    logging.getLogger("docker").propagate = False


def setup_parallel_logger(ansi_mode):
    ParallelStreamWriter.set_default_ansi_mode(ansi_mode)


def setup_console_handler(handler, verbose, use_console_formatter=True, level=None):
    if use_console_formatter:
        format_class = ConsoleWarningFormatter
    else:
        format_class = logging.Formatter

    if verbose:
        handler.setFormatter(format_class('%(name)s.%(funcName)s: %(message)s'))
        loglevel = logging.DEBUG
    else:
        handler.setFormatter(format_class())
        loglevel = logging.INFO

    if level is not None:
        levels = {
            'DEBUG': logging.DEBUG,
            'INFO': logging.INFO,
            'WARNING': logging.WARNING,
            'ERROR': logging.ERROR,
            'CRITICAL': logging.CRITICAL,
        }
        loglevel = levels.get(level.upper())
        if loglevel is None:
            raise UserError(
                'Invalid value for --log-level. Expected one of DEBUG, INFO, WARNING, ERROR, CRITICAL.'
            )

    handler.setLevel(loglevel)


# stolen from docopt master
def parse_doc_section(name, source):
    pattern = re.compile('^([^\n]*' + name + '[^\n]*\n?(?:[ \t].*?(?:\n|$))*)',
                         re.IGNORECASE | re.MULTILINE)
    return [s.strip() for s in pattern.findall(source)]


class TopLevelCommand:
    """Define and run multi-container applications with Docker.

    Usage:
      docker-compose [-f <arg>...] [--profile <name>...] [options] [--] [COMMAND] [ARGS...]
      docker-compose -h|--help

    Options:
      -f, --file FILE             Specify an alternate compose file
                                  (default: docker-compose.yml)
      -p, --project-name NAME     Specify an alternate project name
                                  (default: directory name)
      --profile NAME              Specify a profile to enable
      -c, --context NAME          Specify a context name
      --verbose                   Show more output
      --log-level LEVEL           Set log level (DEBUG, INFO, WARNING, ERROR, CRITICAL)
      --ansi (never|always|auto)  Control when to print ANSI control characters
      --no-ansi                   Do not print ANSI control characters (DEPRECATED)
      -v, --version               Print version and exit
      -H, --host HOST             Daemon socket to connect to

      --tls                       Use TLS; implied by --tlsverify
      --tlscacert CA_PATH         Trust certs signed only by this CA
      --tlscert CLIENT_CERT_PATH  Path to TLS certificate file
      --tlskey TLS_KEY_PATH       Path to TLS key file
      --tlsverify                 Use TLS and verify the remote
      --skip-hostname-check       Don't check the daemon's hostname against the
                                  name specified in the client certificate
      --project-directory PATH    Specify an alternate working directory
                                  (default: the path of the Compose file)
      --compatibility             If set, Compose will attempt to convert keys
                                  in v3 files to their non-Swarm equivalent (DEPRECATED)
      --env-file PATH             Specify an alternate environment file

    Commands:
      build              Build or rebuild services
      config             Validate and view the Compose file
      create             Create services
      down               Stop and remove resources
      events             Receive real time events from containers
      exec               Execute a command in a running container
      help               Get help on a command
      images             List images
      kill               Kill containers
      logs               View output from containers
      pause              Pause services
      port               Print the public port for a port binding
      ps                 List containers
      pull               Pull service images
      push               Push service images
      restart            Restart services
      rm                 Remove stopped containers
      run                Run a one-off command
      scale              Set number of containers for a service
      start              Start services
      stop               Stop services
      top                Display the running processes
      unpause            Unpause services
      up                 Create and start containers
      version            Show version information and quit
    """

    def __init__(self, project, options=None):
        self.project = project
        self.toplevel_options = options or {}

    @property
    def project_dir(self):
        return get_project_dir(self.toplevel_options)

    @property
    def toplevel_environment(self):
        environment_file = self.toplevel_options.get('--env-file')
        return Environment.from_env_file(self.project_dir, environment_file)

    @metrics()
    def build(self, options):
        """
        Build or rebuild services.

        Services are built once and then tagged as `project_service`,
        e.g. `composetest_db`. If you change a service's `Dockerfile` or the
        contents of its build directory, you can run `docker-compose build` to rebuild it.

        Usage: build [options] [--build-arg key=val...] [--] [SERVICE...]

        Options:
            --build-arg key=val     Set build-time variables for services.
            --compress              Compress the build context using gzip.
            --force-rm              Always remove intermediate containers.
            -m, --memory MEM        Set memory limit for the build container.
            --no-cache              Do not use cache when building the image.
            --no-rm                 Do not remove intermediate containers after a successful build.
            --parallel              Build images in parallel.
            --progress string       Set type of progress output (auto, plain, tty).
            --pull                  Always attempt to pull a newer version of the image.
            -q, --quiet             Don't print anything to STDOUT
        """
        service_names = options['SERVICE']
        build_args = options.get('--build-arg', None)
        if build_args:
            if not service_names and docker.utils.version_lt(self.project.client.api_version, '1.25'):
                raise UserError(
                    '--build-arg is only supported when services are specified for API version < 1.25.'
                    ' Please use a Compose file version > 2.2 or specify which services to build.'
                )
            build_args = resolve_build_args(build_args, self.toplevel_environment)

        native_builder = self.toplevel_environment.get_boolean('COMPOSE_DOCKER_CLI_BUILD', True)

        self.project.build(
            service_names=options['SERVICE'],
            no_cache=bool(options.get('--no-cache', False)),
            pull=bool(options.get('--pull', False)),
            force_rm=bool(options.get('--force-rm', False)),
            memory=options.get('--memory'),
            rm=not bool(options.get('--no-rm', False)),
            build_args=build_args,
            gzip=options.get('--compress', False),
            parallel_build=options.get('--parallel', False),
            silent=options.get('--quiet', False),
            cli=native_builder,
            progress=options.get('--progress'),
        )

    @metrics()
    def config(self, options):
        """
        Validate and view the Compose file.

        Usage: config [options]

        Options:
            --resolve-image-digests  Pin image tags to digests.
            --no-interpolate         Don't interpolate environment variables.
            -q, --quiet              Only validate the configuration, don't print
                                     anything.
            --profiles               Print the profile names, one per line.
            --services               Print the service names, one per line.
            --volumes                Print the volume names, one per line.
            --hash="*"               Print the service config hash, one per line.
                                     Set "service1,service2" for a list of specified services
                                     or use the wildcard symbol to display all services.
        """

        additional_options = {'--no-interpolate': options.get('--no-interpolate')}
        compose_config = get_config_from_options('.', self.toplevel_options, additional_options)
        image_digests = None

        if options['--resolve-image-digests']:
            self.project = project_from_options('.', self.toplevel_options, additional_options)
            with errors.handle_connection_errors(self.project.client):
                image_digests = image_digests_for_project(self.project)

        if options['--quiet']:
            return

        if options['--profiles']:
            profiles = set()
            for service in compose_config.services:
                if 'profiles' in service:
                    for profile in service['profiles']:
                        profiles.add(profile)
            print('\n'.join(sorted(profiles)))
            return

        if options['--services']:
            print('\n'.join(service['name'] for service in compose_config.services))
            return

        if options['--volumes']:
            print('\n'.join(volume for volume in compose_config.volumes))
            return

        if options['--hash'] is not None:
            h = options['--hash']
            self.project = project_from_options('.', self.toplevel_options, additional_options)
            services = [svc for svc in options['--hash'].split(',')] if h != '*' else None
            with errors.handle_connection_errors(self.project.client):
                for service in self.project.get_services(services):
                    print('{} {}'.format(service.name, service.config_hash))
            return

        print(serialize_config(compose_config, image_digests, not options['--no-interpolate']))

    @metrics()
    def create(self, options):
        """
        Creates containers for a service.
        This command is deprecated. Use the `up` command with `--no-start` instead.

        Usage: create [options] [SERVICE...]

        Options:
            --force-recreate       Recreate containers even if their configuration and
                                   image haven't changed. Incompatible with --no-recreate.
            --no-recreate          If containers already exist, don't recreate them.
                                   Incompatible with --force-recreate.
            --no-build             Don't build an image, even if it's missing.
            --build                Build images before creating containers.
        """
        service_names = options['SERVICE']

        log.warning(
            'The create command is deprecated. '
            'Use the up command with the --no-start flag instead.'
        )

        self.project.create(
            service_names=service_names,
            strategy=convergence_strategy_from_opts(options),
            do_build=build_action_from_opts(options),
        )

    @metrics()
    def down(self, options):
        """
        Stops containers and removes containers, networks, volumes, and images
        created by `up`.

        By default, the only things removed are:

        - Containers for services defined in the Compose file
        - Networks defined in the `networks` section of the Compose file
        - The default network, if one is used

        Networks and volumes defined as `external` are never removed.

        Usage: down [options]

        Options:
            --rmi type              Remove images. Type must be one of:
                                      'all': Remove all images used by any service.
                                      'local': Remove only images that don't have a
                                      custom tag set by the `image` field.
            -v, --volumes           Remove named volumes declared in the `volumes`
                                    section of the Compose file and anonymous volumes
                                    attached to containers.
            --remove-orphans        Remove containers for services not defined in the
                                    Compose file
            -t, --timeout TIMEOUT   Specify a shutdown timeout in seconds.
                                    (default: 10)
        """
        ignore_orphans = self.toplevel_environment.get_boolean('COMPOSE_IGNORE_ORPHANS')

        if ignore_orphans and options['--remove-orphans']:
            raise UserError("COMPOSE_IGNORE_ORPHANS and --remove-orphans cannot be combined.")

        image_type = image_type_from_opt('--rmi', options['--rmi'])
        timeout = timeout_from_opts(options)
        self.project.down(
            image_type,
            options['--volumes'],
            options['--remove-orphans'],
            timeout=timeout,
            ignore_orphans=ignore_orphans)

    def events(self, options):
        """
        Receive real time events from containers.

        Usage: events [options] [--] [SERVICE...]

        Options:
            --json      Output events as a stream of json objects
        """

        def format_event(event):
            attributes = ["%s=%s" % item for item in event['attributes'].items()]
            return ("{time} {type} {action} {id} ({attrs})").format(
                attrs=", ".join(sorted(attributes)),
                **event)

        def json_format_event(event):
            event['time'] = event['time'].isoformat()
            event.pop('container')
            return json.dumps(event)

        for event in self.project.events():
            formatter = json_format_event if options['--json'] else format_event
            print(formatter(event))
            sys.stdout.flush()

    @metrics("exec")
    def exec_command(self, options):
        """
        Execute a command in a running container

        Usage: exec [options] [-e KEY=VAL...] [--] SERVICE COMMAND [ARGS...]

        Options:
            -d, --detach      Detached mode: Run command in the background.
            --privileged      Give extended privileges to the process.
            -u, --user USER   Run the command as this user.
            -T                Disable pseudo-tty allocation. By default `docker-compose exec`
                              allocates a TTY.
            --index=index     index of the container if there are multiple
                              instances of a service [default: 1]
            -e, --env KEY=VAL Set environment variables (can be used multiple times,
                              not supported in API < 1.25)
            -w, --workdir DIR Path to workdir directory for this command.
        """
        use_cli = not self.toplevel_environment.get_boolean('COMPOSE_INTERACTIVE_NO_CLI')
        index = int(options.get('--index'))
        service = self.project.get_service(options['SERVICE'])
        detach = options.get('--detach')

        if options['--env'] and docker.utils.version_lt(self.project.client.api_version, '1.25'):
            raise UserError("Setting environment for exec is not supported in API < 1.25 (%s)"
                            % self.project.client.api_version)

        if options['--workdir'] and docker.utils.version_lt(self.project.client.api_version, '1.35'):
            raise UserError("Setting workdir for exec is not supported in API < 1.35 (%s)"
                            % self.project.client.api_version)

        try:
            container = service.get_container(number=index)
        except ValueError as e:
            raise UserError(str(e))
        command = [options['COMMAND']] + options['ARGS']
        tty = not options["-T"]

        if IS_WINDOWS_PLATFORM or use_cli and not detach:
            sys.exit(call_docker(
                build_exec_command(options, container.id, command),
                self.toplevel_options, self.toplevel_environment)
            )

        create_exec_options = {
            "privileged": options["--privileged"],
            "user": options["--user"],
            "tty": tty,
            "stdin": True,
            "workdir": options["--workdir"],
        }

        if docker.utils.version_gte(self.project.client.api_version, '1.25'):
            create_exec_options["environment"] = options["--env"]

        exec_id = container.create_exec(command, **create_exec_options)

        if detach:
            container.start_exec(exec_id, tty=tty, stream=True)
            return

        signals.set_signal_handler_to_shutdown()
        try:
            operation = ExecOperation(
                self.project.client,
                exec_id,
                interactive=tty,
            )
            pty = PseudoTerminal(self.project.client, operation)
            pty.start()
        except signals.ShutdownException:
            log.info("received shutdown exception: closing")
        exit_code = self.project.client.exec_inspect(exec_id).get("ExitCode")
        sys.exit(exit_code)

    @classmethod
    @metrics()
    def help(cls, options):
        """
        Get help on a command.

        Usage: help [COMMAND]
        """
        if options['COMMAND']:
            subject = get_handler(cls, options['COMMAND'])
        else:
            subject = cls

        print(getdoc(subject))

    @metrics()
    def images(self, options):
        """
        List images used by the created containers.
        Usage: images [options] [--] [SERVICE...]

        Options:
            -q, --quiet  Only display IDs
        """
        containers = sorted(
            self.project.containers(service_names=options['SERVICE'], stopped=True) +
            self.project.containers(service_names=options['SERVICE'], one_off=OneOffFilter.only),
            key=attrgetter('name'))

        if options['--quiet']:
            for image in {c.image for c in containers}:
                print(image.split(':')[1])
            return

        def add_default_tag(img_name):
            if ':' not in img_name.split('/')[-1]:
                return '{}:latest'.format(img_name)
            return img_name

        headers = [
            'Container',
            'Repository',
            'Tag',
            'Image Id',
            'Size'
        ]
        rows = []
        for container in containers:
            image_config = container.image_config
            service = self.project.get_service(container.service)
            index = 0
            img_name = add_default_tag(service.image_name)
            if img_name in image_config['RepoTags']:
                index = image_config['RepoTags'].index(img_name)
            repo_tags = (
                image_config['RepoTags'][index].rsplit(':', 1) if image_config['RepoTags']
                else ('<none>', '<none>')
            )

            image_id = image_config['Id'].split(':')[1][:12]
            size = human_readable_file_size(image_config['Size'])
            rows.append([
                container.name,
                repo_tags[0],
                repo_tags[1],
                image_id,
                size
            ])
        print(Formatter.table(headers, rows))

    @metrics()
    def kill(self, options):
        """
        Force stop service containers.

        Usage: kill [options] [--] [SERVICE...]

        Options:
            -s SIGNAL         SIGNAL to send to the container.
                              Default signal is SIGKILL.
        """
        signal = options.get('-s', 'SIGKILL')

        self.project.kill(service_names=options['SERVICE'], signal=signal)

    @metrics()
    def logs(self, options):
        """
        View output from containers.

        Usage: logs [options] [--] [SERVICE...]

        Options:
            --no-color              Produce monochrome output.
            -f, --follow            Follow log output.
            -t, --timestamps        Show timestamps.
            --tail="all"            Number of lines to show from the end of the logs
                                    for each container.
            --no-log-prefix         Don't print prefix in logs.
        """
        containers = self.project.containers(service_names=options['SERVICE'], stopped=True)

        tail = options['--tail']
        if tail is not None:
            if tail.isdigit():
                tail = int(tail)
            elif tail != 'all':
                raise UserError("tail flag must be all or a number")
        log_args = {
            'follow': options['--follow'],
            'tail': tail,
            'timestamps': options['--timestamps']
        }
        print("Attaching to", list_containers(containers))
        log_printer_from_project(
            self.project,
            containers,
            options['--no-color'],
            log_args,
            event_stream=self.project.events(service_names=options['SERVICE']),
            keep_prefix=not options['--no-log-prefix']).run()

    @metrics()
    def pause(self, options):
        """
        Pause services.

        Usage: pause [SERVICE...]
        """
        containers = self.project.pause(service_names=options['SERVICE'])
        exit_if(not containers, 'No containers to pause', 1)

    @metrics()
    def port(self, options):
        """
        Print the public port for a port binding.

        Usage: port [options] [--] SERVICE PRIVATE_PORT

        Options:
            --protocol=proto  tcp or udp [default: tcp]
            --index=index     index of the container if there are multiple
                              instances of a service [default: 1]
        """
        index = int(options.get('--index'))
        service = self.project.get_service(options['SERVICE'])
        try:
            container = service.get_container(number=index)
        except ValueError as e:
            raise UserError(str(e))
        print(container.get_local_port(
            options['PRIVATE_PORT'],
            protocol=options.get('--protocol') or 'tcp') or '')

    @metrics()
    def ps(self, options):
        """
        List containers.

        Usage: ps [options] [--] [SERVICE...]

        Options:
            -q, --quiet          Only display IDs
            --services           Display services
            --filter KEY=VAL     Filter services by a property. KEY is either:
                                 1. `source` with values `image`, or `build`;
                                 2. `status` with values `running`, `stopped`, `paused`, or `restarted`.
            -a, --all            Show all stopped containers (including those created by the run command)
        """
        if options['--quiet'] and options['--services']:
            raise UserError('--quiet and --services cannot be combined')

        if options['--services']:
            filt = build_filter(options.get('--filter'))
            services = self.project.services
            if filt:
                services = filter_services(filt, services, self.project)
            print('\n'.join(service.name for service in services))
            return

        if options['--all']:
            containers = sorted(self.project.containers(service_names=options['SERVICE'],
                                                        one_off=OneOffFilter.include, stopped=True),
                                key=attrgetter('name'))
        else:
            containers = sorted(
                self.project.containers(service_names=options['SERVICE'], stopped=True) +
                self.project.containers(service_names=options['SERVICE'], one_off=OneOffFilter.only),
                key=attrgetter('name'))

        if options['--quiet']:
            for container in containers:
                print(container.id)
        else:
            headers = [
                'Name',
                'Command',
                'State',
                'Ports',
            ]
            rows = []
            for container in containers:
                command = container.human_readable_command
                if len(command) > 30:
                    command = '%s ...' % command[:26]
                rows.append([
                    container.name,
                    command,
                    container.human_readable_state,
                    container.human_readable_ports,
                ])
            print(Formatter.table(headers, rows))

    @metrics()
    def pull(self, options):
        """
        Pulls images for services defined in a Compose file, but does not start the containers.

        Usage: pull [options] [--] [SERVICE...]

        Options:
            --ignore-pull-failures  Pull what it can and ignores images with pull failures.
            --parallel              Deprecated, pull multiple images in parallel (enabled by default).
            --no-parallel           Disable parallel pulling.
            -q, --quiet             Pull without printing progress information
            --include-deps          Also pull services declared as dependencies
        """
        if options.get('--parallel'):
            log.warning('--parallel option is deprecated and will be removed in future versions.')
        self.project.pull(
            service_names=options['SERVICE'],
            ignore_pull_failures=options.get('--ignore-pull-failures'),
            parallel_pull=not options.get('--no-parallel'),
            silent=options.get('--quiet'),
            include_deps=options.get('--include-deps'),
        )

    @metrics()
    def push(self, options):
        """
        Pushes images for services.

        Usage: push [options] [--] [SERVICE...]

        Options:
            --ignore-push-failures  Push what it can and ignores images with push failures.
        """
        self.project.push(
            service_names=options['SERVICE'],
            ignore_push_failures=options.get('--ignore-push-failures')
        )

    @metrics()
    def rm(self, options):
        """
        Removes stopped service containers.

        By default, anonymous volumes attached to containers will not be removed. You
        can override this with `-v`. To list all volumes, use `docker volume ls`.

        Any data which is not in a volume will be lost.

        Usage: rm [options] [--] [SERVICE...]

        Options:
            -f, --force   Don't ask to confirm removal
            -s, --stop    Stop the containers, if required, before removing
            -v            Remove any anonymous volumes attached to containers
            -a, --all     Deprecated - no effect.
        """
        if options.get('--all'):
            log.warning(
                '--all flag is obsolete. This is now the default behavior '
                'of `docker-compose rm`'
            )
        one_off = OneOffFilter.include

        if options.get('--stop'):
            self.project.stop(service_names=options['SERVICE'], one_off=one_off)

        all_containers = self.project.containers(
            service_names=options['SERVICE'], stopped=True, one_off=one_off
        )
        stopped_containers = [c for c in all_containers if not c.is_running]

        if len(stopped_containers) > 0:
            print("Going to remove", list_containers(stopped_containers))
            if options.get('--force') \
                    or yesno("Are you sure? [yN] ", default=False):
                self.project.remove_stopped(
                    service_names=options['SERVICE'],
                    v=options.get('-v', False),
                    one_off=one_off
                )
        else:
            print("No stopped containers")

    @metrics()
    def run(self, options):
        """
        Run a one-off command on a service.

        For example:

            $ docker-compose run web python manage.py shell

        By default, linked services will be started, unless they are already
        running. If you do not want to start linked services, use
        `docker-compose run --no-deps SERVICE COMMAND [ARGS...]`.

        Usage:
            run [options] [-v VOLUME...] [-p PORT...] [-e KEY=VAL...] [-l KEY=VALUE...] [--]
                SERVICE [COMMAND] [ARGS...]

        Options:
            -d, --detach          Detached mode: Run container in the background, print
                                  new container name.
            --name NAME           Assign a name to the container
            --entrypoint CMD      Override the entrypoint of the image.
            -e KEY=VAL            Set an environment variable (can be used multiple times)
            -l, --label KEY=VAL   Add or override a label (can be used multiple times)
            -u, --user=""         Run as specified username or uid
            --no-deps             Don't start linked services.
            --rm                  Remove container after run. Ignored in detached mode.
            -p, --publish=[]      Publish a container's port(s) to the host
            --service-ports       Run command with the service's ports enabled and mapped
                                  to the host.
            --use-aliases         Use the service's network aliases in the network(s) the
                                  container connects to.
            -v, --volume=[]       Bind mount a volume (default [])
            -T                    Disable pseudo-tty allocation. By default `docker-compose run`
                                  allocates a TTY.
            -w, --workdir=""      Working directory inside the container
        """
        service = self.project.get_service(options['SERVICE'])
        detach = options.get('--detach')

        if options['--publish'] and options['--service-ports']:
            raise UserError(
                'Service port mapping and manual port mapping '
                'can not be used together'
            )

        if options['COMMAND'] is not None:
            command = [options['COMMAND']] + options['ARGS']
        elif options['--entrypoint'] is not None:
            command = []
        else:
            command = service.options.get('command')

        options['stdin_open'] = service.options.get('stdin_open', True)

        container_options = build_one_off_container_options(options, detach, command)
        run_one_off_container(
            container_options, self.project, service, options,
            self.toplevel_options, self.toplevel_environment
        )

    @metrics()
    def scale(self, options):
        """
        Set number of containers to run for a service.

        Numbers are specified in the form `service=num` as arguments.
        For example:

            $ docker-compose scale web=2 worker=3

        This command is deprecated. Use the up command with the `--scale` flag
        instead.

        Usage: scale [options] [SERVICE=NUM...]

        Options:
          -t, --timeout TIMEOUT      Specify a shutdown timeout in seconds.
                                     (default: 10)
        """
        timeout = timeout_from_opts(options)

        log.warning(
            'The scale command is deprecated. '
            'Use the up command with the --scale flag instead.'
        )

        for service_name, num in parse_scale_args(options['SERVICE=NUM']).items():
            self.project.get_service(service_name).scale(num, timeout=timeout)

    @metrics()
    def start(self, options):
        """
        Start existing containers.

        Usage: start [SERVICE...]
        """
        containers = self.project.start(service_names=options['SERVICE'])
        exit_if(not containers, 'No containers to start', 1)

    @metrics()
    def stop(self, options):
        """
        Stop running containers without removing them.

        They can be started again with `docker-compose start`.

        Usage: stop [options] [--] [SERVICE...]

        Options:
          -t, --timeout TIMEOUT      Specify a shutdown timeout in seconds.
                                     (default: 10)
        """
        timeout = timeout_from_opts(options)
        self.project.stop(service_names=options['SERVICE'], timeout=timeout)

    @metrics()
    def restart(self, options):
        """
        Restart running containers.

        Usage: restart [options] [--] [SERVICE...]

        Options:
          -t, --timeout TIMEOUT      Specify a shutdown timeout in seconds.
                                     (default: 10)
        """
        timeout = timeout_from_opts(options)
        containers = self.project.restart(service_names=options['SERVICE'], timeout=timeout)
        exit_if(not containers, 'No containers to restart', 1)

    @metrics()
    def top(self, options):
        """
        Display the running processes

        Usage: top [SERVICE...]

        """
        containers = sorted(
            self.project.containers(service_names=options['SERVICE'], stopped=False) +
            self.project.containers(service_names=options['SERVICE'], one_off=OneOffFilter.only),
            key=attrgetter('name')
        )

        for idx, container in enumerate(containers):
            if idx > 0:
                print()

            top_data = self.project.client.top(container.name)
            headers = top_data.get("Titles")
            rows = []

            for process in top_data.get("Processes", []):
                rows.append(process)

            print(container.name)
            print(Formatter.table(headers, rows))

    @metrics()
    def unpause(self, options):
        """
        Unpause services.

        Usage: unpause [SERVICE...]
        """
        containers = self.project.unpause(service_names=options['SERVICE'])
        exit_if(not containers, 'No containers to unpause', 1)

    @metrics()
    def up(self, options):
        """
        Builds, (re)creates, starts, and attaches to containers for a service.

        Unless they are already running, this command also starts any linked services.

        The `docker-compose up` command aggregates the output of each container. When
        the command exits, all containers are stopped. Running `docker-compose up -d`
        starts the containers in the background and leaves them running.

        If there are existing containers for a service, and the service's configuration
        or image was changed after the container's creation, `docker-compose up` picks
        up the changes by stopping and recreating the containers (preserving mounted
        volumes). To prevent Compose from picking up changes, use the `--no-recreate`
        flag.

        If you want to force Compose to stop and recreate all containers, use the
        `--force-recreate` flag.

        Usage: up [options] [--scale SERVICE=NUM...] [--] [SERVICE...]

        Options:
            -d, --detach               Detached mode: Run containers in the background,
                                       print new container names. Incompatible with
                                       --abort-on-container-exit.
            --no-color                 Produce monochrome output.
            --quiet-pull               Pull without printing progress information
            --no-deps                  Don't start linked services.
            --force-recreate           Recreate containers even if their configuration
                                       and image haven't changed.
            --always-recreate-deps     Recreate dependent containers.
                                       Incompatible with --no-recreate.
            --no-recreate              If containers already exist, don't recreate
                                       them. Incompatible with --force-recreate and -V.
            --no-build                 Don't build an image, even if it's missing.
            --no-start                 Don't start the services after creating them.
            --build                    Build images before starting containers.
            --abort-on-container-exit  Stops all containers if any container was
                                       stopped. Incompatible with -d.
            --attach-dependencies      Attach to dependent containers.
            -t, --timeout TIMEOUT      Use this timeout in seconds for container
                                       shutdown when attached or when containers are
                                       already running. (default: 10)
            -V, --renew-anon-volumes   Recreate anonymous volumes instead of retrieving
                                       data from the previous containers.
            --remove-orphans           Remove containers for services not defined
                                       in the Compose file.
            --exit-code-from SERVICE   Return the exit code of the selected service
                                       container. Implies --abort-on-container-exit.
            --scale SERVICE=NUM        Scale SERVICE to NUM instances. Overrides the
                                       `scale` setting in the Compose file if present.
            --no-log-prefix            Don't print prefix in logs.
        """
        start_deps = not options['--no-deps']
        always_recreate_deps = options['--always-recreate-deps']
        exit_value_from = exitval_from_opts(options, self.project)
        cascade_stop = options['--abort-on-container-exit']
        service_names = options['SERVICE']
        timeout = timeout_from_opts(options)
        remove_orphans = options['--remove-orphans']
        detached = options.get('--detach')
        no_start = options.get('--no-start')
        attach_dependencies = options.get('--attach-dependencies')
        keep_prefix = not options.get('--no-log-prefix')

        if detached and (cascade_stop or exit_value_from or attach_dependencies):
            raise UserError(
                "-d cannot be combined with --abort-on-container-exit or --attach-dependencies.")

        ignore_orphans = self.toplevel_environment.get_boolean('COMPOSE_IGNORE_ORPHANS')

        if ignore_orphans and remove_orphans:
            raise UserError("COMPOSE_IGNORE_ORPHANS and --remove-orphans cannot be combined.")

        opts = ['--detach', '--abort-on-container-exit', '--exit-code-from', '--attach-dependencies']
        for excluded in [x for x in opts if options.get(x) and no_start]:
            raise UserError('--no-start and {} cannot be combined.'.format(excluded))

        native_builder = self.toplevel_environment.get_boolean('COMPOSE_DOCKER_CLI_BUILD', True)

        with up_shutdown_context(self.project, service_names, timeout, detached):
            warn_for_swarm_mode(self.project.client)

            def up(rebuild):
                return self.project.up(
                    service_names=service_names,
                    start_deps=start_deps,
                    strategy=convergence_strategy_from_opts(options),
                    do_build=build_action_from_opts(options),
                    timeout=timeout,
                    detached=detached,
                    remove_orphans=remove_orphans,
                    ignore_orphans=ignore_orphans,
                    scale_override=parse_scale_args(options['--scale']),
                    start=not no_start,
                    always_recreate_deps=always_recreate_deps,
                    reset_container_image=rebuild,
                    renew_anonymous_volumes=options.get('--renew-anon-volumes'),
                    silent=options.get('--quiet-pull'),
                    cli=native_builder,
                    attach_dependencies=attach_dependencies,
                )

            try:
                to_attach = up(False)
            except docker.errors.ImageNotFound as e:
                log.error(
                    "The image for the service you're trying to recreate has been removed. "
                    "If you continue, volume data could be lost. Consider backing up your data "
                    "before continuing.\n"
                )
                res = yesno("Continue with the new image? [yN]", False)
                if res is None or not res:
                    raise e

                to_attach = up(True)

            if detached or no_start:
                return

            attached_containers = filter_attached_containers(
                to_attach,
                service_names,
                attach_dependencies)

            log_printer = log_printer_from_project(
                self.project,
                attached_containers,
                options['--no-color'],
                {'follow': True},
                cascade_stop,
                event_stream=self.project.events(service_names=service_names),
                keep_prefix=keep_prefix)
            print("Attaching to", list_containers(log_printer.containers))
            cascade_starter = log_printer.run()

            if cascade_stop:
                print("Aborting on container exit...")
                all_containers = self.project.containers(service_names=options['SERVICE'], stopped=True)
                exit_code = compute_exit_code(
                    exit_value_from, attached_containers, cascade_starter, all_containers
                )

                self.project.stop(service_names=service_names, timeout=timeout)
                if exit_value_from:
                    exit_code = compute_service_exit_code(exit_value_from, attached_containers)

                sys.exit(exit_code)

    @classmethod
    @metrics()
    def version(cls, options):
        """
        Show version information and quit.

        Usage: version [--short]

        Options:
            --short     Shows only Compose's version number.
        """
        if options['--short']:
            print(__version__)
        else:
            print(get_version_info('full'))


def compute_service_exit_code(exit_value_from, attached_containers):
    candidates = list(filter(
        lambda c: c.service == exit_value_from,
        attached_containers))
    if not candidates:
        log.error(
            'No containers matching the spec "{}" '
            'were run.'.format(exit_value_from)
        )
        return 2
    if len(candidates) > 1:
        exit_values = filter(
            lambda e: e != 0,
            [c.inspect()['State']['ExitCode'] for c in candidates]
        )

        return exit_values[0]
    return candidates[0].inspect()['State']['ExitCode']


def compute_exit_code(exit_value_from, attached_containers, cascade_starter, all_containers):
    exit_code = 0
    for e in all_containers:
        if (not e.is_running and cascade_starter == e.name):
            if not e.exit_code == 0:
                exit_code = e.exit_code
                break

    return exit_code


def convergence_strategy_from_opts(options):
    no_recreate = options['--no-recreate']
    force_recreate = options['--force-recreate']
    renew_anonymous_volumes = options.get('--renew-anon-volumes')
    if force_recreate and no_recreate:
        raise UserError("--force-recreate and --no-recreate cannot be combined.")

    if no_recreate and renew_anonymous_volumes:
        raise UserError('--no-recreate and --renew-anon-volumes cannot be combined.')

    if force_recreate or renew_anonymous_volumes:
        return ConvergenceStrategy.always

    if no_recreate:
        return ConvergenceStrategy.never

    return ConvergenceStrategy.changed


def timeout_from_opts(options):
    timeout = options.get('--timeout')
    return None if timeout is None else int(timeout)


def image_digests_for_project(project):
    try:
        return get_image_digests(project)

    except MissingDigests as e:
        def list_images(images):
            return "\n".join("    {}".format(name) for name in sorted(images))

        paras = ["Some images are missing digests."]

        if e.needs_push:
            command_hint = (
                "Use `docker push {}` to push them. "
                .format(" ".join(sorted(e.needs_push)))
            )
            paras += [
                "The following images can be pushed:",
                list_images(e.needs_push),
                command_hint,
            ]

        if e.needs_pull:
            command_hint = (
                "Use `docker pull {}` to pull them. "
                .format(" ".join(sorted(e.needs_pull)))
            )

            paras += [
                "The following images need to be pulled:",
                list_images(e.needs_pull),
                command_hint,
            ]

        raise UserError("\n\n".join(paras))


def exitval_from_opts(options, project):
    exit_value_from = options.get('--exit-code-from')
    if exit_value_from:
        if not options.get('--abort-on-container-exit'):
            log.warning('using --exit-code-from implies --abort-on-container-exit')
            options['--abort-on-container-exit'] = True
        if exit_value_from not in [s.name for s in project.get_services()]:
            log.error('No service named "%s" was found in your compose file.',
                      exit_value_from)
            sys.exit(2)
    return exit_value_from


def image_type_from_opt(flag, value):
    if not value:
        return ImageType.none
    try:
        return ImageType[value]
    except KeyError:
        raise UserError("%s flag must be one of: all, local" % flag)


def build_action_from_opts(options):
    if options['--build'] and options['--no-build']:
        raise UserError("--build and --no-build can not be combined.")

    if options['--build']:
        return BuildAction.force

    if options['--no-build']:
        return BuildAction.skip

    return BuildAction.none


def build_one_off_container_options(options, detach, command):
    container_options = {
        'command': command,
        'tty': not (detach or options['-T'] or not sys.stdin.isatty()),
        'stdin_open': options.get('stdin_open'),
        'detach': detach,
    }

    if options['-e']:
        container_options['environment'] = Environment.from_command_line(
            parse_environment(options['-e'])
        )

    if options['--label']:
        container_options['labels'] = parse_labels(options['--label'])

    if options.get('--entrypoint') is not None:
        container_options['entrypoint'] = (
            [""] if options['--entrypoint'] == '' else options['--entrypoint']
        )

    # Ensure that run command remains one-off (issue #6302)
    container_options['restart'] = None

    if options['--user']:
        container_options['user'] = options.get('--user')

    if not options['--service-ports']:
        container_options['ports'] = []

    if options['--publish']:
        container_options['ports'] = options.get('--publish')

    if options['--name']:
        container_options['name'] = options['--name']

    if options['--workdir']:
        container_options['working_dir'] = options['--workdir']

    if options['--volume']:
        volumes = [VolumeSpec.parse(i) for i in options['--volume']]
        container_options['volumes'] = volumes

    return container_options


def run_one_off_container(container_options, project, service, options, toplevel_options,
                          toplevel_environment):
    native_builder = toplevel_environment.get_boolean('COMPOSE_DOCKER_CLI_BUILD')
    detach = options.get('--detach')
    use_network_aliases = options.get('--use-aliases')
    service.scale_num = 1
    containers = project.up(
        service_names=[service.name],
        start_deps=not options['--no-deps'],
        strategy=ConvergenceStrategy.never,
        detached=True,
        rescale=False,
        cli=native_builder,
        one_off=True,
        override_options=container_options,
    )
    try:
        container = next(c for c in containers if c.service == service.name)
    except StopIteration:
        raise OperationFailedError('Could not bring up the requested service')

    if detach:
        service.start_container(container, use_network_aliases)
        print(container.name)
        return

    def remove_container():
        if options['--rm']:
            project.client.remove_container(container.id, force=True, v=True)

    use_cli = not toplevel_environment.get_boolean('COMPOSE_INTERACTIVE_NO_CLI')

    signals.set_signal_handler_to_shutdown()
    signals.set_signal_handler_to_hang_up()
    try:
        try:
            if IS_WINDOWS_PLATFORM or use_cli:
                service.connect_container_to_networks(container, use_network_aliases)
                exit_code = call_docker(
                    get_docker_start_call(container_options, container.id),
                    toplevel_options, toplevel_environment
                )
            else:
                operation = RunOperation(
                    project.client,
                    container.id,
                    interactive=not options['-T'],
                    logs=False,
                )
                pty = PseudoTerminal(project.client, operation)
                sockets = pty.sockets()
                service.start_container(container, use_network_aliases)
                pty.start(sockets)
                exit_code = container.wait()
        except (signals.ShutdownException):
            project.client.stop(container.id)
            exit_code = 1
    except (signals.ShutdownException, signals.HangUpException):
        project.client.kill(container.id)
        remove_container()
        sys.exit(2)

    remove_container()
    sys.exit(exit_code)


def get_docker_start_call(container_options, container_id):
    docker_call = ["start"]
    if not container_options.get('detach'):
        docker_call.append("--attach")
    if container_options.get('stdin_open'):
        docker_call.append("--interactive")
    docker_call.append(container_id)
    return docker_call


def log_printer_from_project(
        project,
        containers,
        monochrome,
        log_args,
        cascade_stop=False,
        event_stream=None,
        keep_prefix=True,
):
    return LogPrinter(
        [c for c in containers if c.log_driver not in (None, 'none')],
        build_log_presenters(project.service_names, monochrome, keep_prefix),
        event_stream or project.events(),
        cascade_stop=cascade_stop,
        log_args=log_args)


def filter_attached_containers(containers, service_names, attach_dependencies=False):
    return filter_attached_for_up(
        containers,
        service_names,
        attach_dependencies,
        lambda container: container.service)


@contextlib.contextmanager
def up_shutdown_context(project, service_names, timeout, detached):
    if detached:
        yield
        return

    signals.set_signal_handler_to_shutdown()
    try:
        try:
            yield
        except signals.ShutdownException:
            print("Gracefully stopping... (press Ctrl+C again to force)")
            project.stop(service_names=service_names, timeout=timeout)
    except signals.ShutdownException:
        project.kill(service_names=service_names)
        sys.exit(2)


def list_containers(containers):
    return ", ".join(c.name for c in containers)


def exit_if(condition, message, exit_code):
    if condition:
        log.error(message)
        raise SystemExit(exit_code)


def call_docker(args, dockeropts, environment):
    executable_path = find_executable('docker')
    if not executable_path:
        raise UserError(errors.docker_not_found_msg("Couldn't find `docker` binary."))

    tls = dockeropts.get('--tls', False)
    ca_cert = dockeropts.get('--tlscacert')
    cert = dockeropts.get('--tlscert')
    key = dockeropts.get('--tlskey')
    verify = dockeropts.get('--tlsverify')
    host = dockeropts.get('--host')
    context = dockeropts.get('--context')
    tls_options = []
    if tls:
        tls_options.append('--tls')
    if ca_cert:
        tls_options.extend(['--tlscacert', ca_cert])
    if cert:
        tls_options.extend(['--tlscert', cert])
    if key:
        tls_options.extend(['--tlskey', key])
    if verify:
        tls_options.append('--tlsverify')
    if host:
        tls_options.extend(
            ['--host', re.sub(r'^https?://', 'tcp://', host.lstrip('='))]
        )
    if context:
        tls_options.extend(
            ['--context', context]
        )

    args = [executable_path] + tls_options + args
    log.debug(" ".join(map(shlex.quote, args)))

    filtered_env = {k: v for k, v in environment.items() if v is not None}

    return subprocess.call(args, env=filtered_env)


def parse_scale_args(options):
    res = {}
    for s in options:
        if '=' not in s:
            raise UserError('Arguments to scale should be in the form service=num')
        service_name, num = s.split('=', 1)
        try:
            num = int(num)
        except ValueError:
            raise UserError(
                'Number of containers for service "%s" is not a number' % service_name
            )
        res[service_name] = num
    return res


def build_exec_command(options, container_id, command):
    args = ["exec"]

    if options["--detach"]:
        args += ["--detach"]
    else:
        args += ["--interactive"]

    if not options["-T"]:
        args += ["--tty"]

    if options["--privileged"]:
        args += ["--privileged"]

    if options["--user"]:
        args += ["--user", options["--user"]]

    if options["--env"]:
        for env_variable in options["--env"]:
            args += ["--env", env_variable]

    if options["--workdir"]:
        args += ["--workdir", options["--workdir"]]

    args += [container_id]
    args += command
    return args


def has_container_with_state(containers, state):
    states = {
        'running': lambda c: c.is_running,
        'stopped': lambda c: not c.is_running,
        'paused': lambda c: c.is_paused,
        'restarting': lambda c: c.is_restarting,
    }
    for container in containers:
        if state not in states:
            raise UserError("Invalid state: %s" % state)
        if states[state](container):
            return True


def filter_services(filt, services, project):
    def should_include(service):
        for f in filt:
            if f == 'status':
                state = filt[f]
                containers = project.containers([service.name], stopped=True)
                if not has_container_with_state(containers, state):
                    return False
            elif f == 'source':
                source = filt[f]
                if source == 'image' or source == 'build':
                    if source not in service.options:
                        return False
                else:
                    raise UserError("Invalid value for source filter: %s" % source)
            else:
                raise UserError("Invalid filter: %s" % f)
        return True

    return filter(should_include, services)


def build_filter(arg):
    filt = {}
    if arg is not None:
        if '=' not in arg:
            raise UserError("Arguments to --filter should be in form KEY=VAL")
        key, val = arg.split('=', 1)
        filt[key] = val
    return filt


def warn_for_swarm_mode(client):
    info = client.info()
    if info.get('Swarm', {}).get('LocalNodeState') == 'active':
        if info.get('ServerVersion', '').startswith('ucp'):
            # UCP does multi-node scheduling with traditional Compose files.
            return

        log.warning(
            "The Docker Engine you're using is running in swarm mode.\n\n"
            "Compose does not use swarm mode to deploy services to multiple nodes in a swarm. "
            "All containers will be scheduled on the current node.\n\n"
            "To deploy your application across the swarm, "
            "use `docker stack deploy`.\n"
        )
