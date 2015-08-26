from __future__ import print_function
from __future__ import unicode_literals

import logging
import re
import signal
import sys
from inspect import getdoc
from operator import attrgetter

import dockerpty
from docker.errors import APIError

from .. import __version__
from .. import legacy
from ..config import parse_environment
from ..const import DEFAULT_TIMEOUT
from ..progress_stream import StreamOutputError
from ..project import ConfigurationError
from ..project import NoSuchService
from ..service import BuildError
from ..service import NeedsBuildError
from .command import Command
from .docopt_command import NoSuchCommand
from .errors import UserError
from .formatter import Formatter
from .log_printer import LogPrinter
from .utils import get_version_info
from .utils import yesno

log = logging.getLogger(__name__)
console_handler = logging.StreamHandler(sys.stderr)

INSECURE_SSL_WARNING = """
Warning: --allow-insecure-ssl is deprecated and has no effect.
It will be removed in a future version of Compose.
"""


def main():
    setup_logging()
    try:
        command = TopLevelCommand()
        command.sys_dispatch()
    except KeyboardInterrupt:
        log.error("\nAborting.")
        sys.exit(1)
    except (UserError, NoSuchService, ConfigurationError, legacy.LegacyError) as e:
        log.error(e.msg)
        sys.exit(1)
    except NoSuchCommand as e:
        log.error("No such command: %s", e.command)
        log.error("")
        log.error("\n".join(parse_doc_section("commands:", getdoc(e.supercommand))))
        sys.exit(1)
    except APIError as e:
        log.error(e.explanation)
        sys.exit(1)
    except BuildError as e:
        log.error("Service '%s' failed to build: %s" % (e.service.name, e.reason))
        sys.exit(1)
    except StreamOutputError as e:
        log.error(e)
        sys.exit(1)
    except NeedsBuildError as e:
        log.error("Service '%s' needs to be built, but --no-build was passed." % e.service.name)
        sys.exit(1)


def setup_logging():
    root_logger = logging.getLogger()
    root_logger.addHandler(console_handler)
    root_logger.setLevel(logging.DEBUG)

    # Disable requests logging
    logging.getLogger("requests").propagate = False


# stolen from docopt master
def parse_doc_section(name, source):
    pattern = re.compile('^([^\n]*' + name + '[^\n]*\n?(?:[ \t].*?(?:\n|$))*)',
                         re.IGNORECASE | re.MULTILINE)
    return [s.strip() for s in pattern.findall(source)]


class TopLevelCommand(Command):
    """Define and run multi-container applications with Docker.

    Usage:
      docker-compose [options] [COMMAND] [ARGS...]
      docker-compose -h|--help

    Options:
      -f, --file FILE           Specify an alternate compose file (default: docker-compose.yml)
      -p, --project-name NAME   Specify an alternate project name (default: directory name)
      --verbose                 Show more output
      -v, --version             Print version and exit

    Commands:
      build              Build or rebuild services
      help               Get help on a command
      kill               Kill containers
      logs               View output from containers
      port               Print the public port for a port binding
      ps                 List containers
      pull               Pulls service images
      restart            Restart services
      rm                 Remove stopped containers
      run                Run a one-off command
      scale              Set number of containers for a service
      start              Start services
      stop               Stop services
      up                 Create and start containers
      migrate-to-labels  Recreate containers to add labels
      version            Show the Docker-Compose version information

    """
    def docopt_options(self):
        options = super(TopLevelCommand, self).docopt_options()
        options['version'] = get_version_info('compose')
        return options

    def perform_command(self, options, *args, **kwargs):
        if options.get('--verbose'):
            console_handler.setFormatter(logging.Formatter('%(name)s.%(funcName)s: %(message)s'))
            console_handler.setLevel(logging.DEBUG)
        else:
            console_handler.setFormatter(logging.Formatter())
            console_handler.setLevel(logging.INFO)

        return super(TopLevelCommand, self).perform_command(options, *args, **kwargs)

    def build(self, project, options):
        """
        Build or rebuild services.

        Services are built once and then tagged as `project_service`,
        e.g. `composetest_db`. If you change a service's `Dockerfile` or the
        contents of its build directory, you can run `docker-compose build` to rebuild it.

        Usage: build [options] [SERVICE...]

        Options:
            --no-cache  Do not use cache when building the image.
        """
        no_cache = bool(options.get('--no-cache', False))
        project.build(service_names=options['SERVICE'], no_cache=no_cache)

    def help(self, project, options):
        """
        Get help on a command.

        Usage: help COMMAND
        """
        handler = self.get_handler(options['COMMAND'])
        raise SystemExit(getdoc(handler))

    def kill(self, project, options):
        """
        Force stop service containers.

        Usage: kill [options] [SERVICE...]

        Options:
            -s SIGNAL         SIGNAL to send to the container.
                              Default signal is SIGKILL.
        """
        signal = options.get('-s', 'SIGKILL')

        project.kill(service_names=options['SERVICE'], signal=signal)

    def logs(self, project, options):
        """
        View output from containers.

        Usage: logs [options] [SERVICE...]

        Options:
            --no-color  Produce monochrome output.
        """
        containers = project.containers(service_names=options['SERVICE'], stopped=True)

        monochrome = options['--no-color']
        print("Attaching to", list_containers(containers))
        LogPrinter(containers, attach_params={'logs': True}, monochrome=monochrome).run()

    def pause(self, project, options):
        """
        Pause services.

        Usage: pause [SERVICE...]
        """
        project.pause(service_names=options['SERVICE'])

    def port(self, project, options):
        """
        Print the public port for a port binding.

        Usage: port [options] SERVICE PRIVATE_PORT

        Options:
            --protocol=proto  tcp or udp [default: tcp]
            --index=index     index of the container if there are multiple
                              instances of a service [default: 1]
        """
        index = int(options.get('--index'))
        service = project.get_service(options['SERVICE'])
        try:
            container = service.get_container(number=index)
        except ValueError as e:
            raise UserError(str(e))
        print(container.get_local_port(
            options['PRIVATE_PORT'],
            protocol=options.get('--protocol') or 'tcp') or '')

    def ps(self, project, options):
        """
        List containers.

        Usage: ps [options] [SERVICE...]

        Options:
            -q    Only display IDs
        """
        containers = sorted(
            project.containers(service_names=options['SERVICE'], stopped=True) +
            project.containers(service_names=options['SERVICE'], one_off=True),
            key=attrgetter('name'))

        if options['-q']:
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
            print(Formatter().table(headers, rows))

    def pull(self, project, options):
        """
        Pulls images for services.

        Usage: pull [options] [SERVICE...]

        Options:
            --allow-insecure-ssl    Deprecated - no effect.
        """
        if options['--allow-insecure-ssl']:
            log.warn(INSECURE_SSL_WARNING)

        project.pull(
            service_names=options['SERVICE'],
        )

    def rm(self, project, options):
        """
        Remove stopped service containers.

        Usage: rm [options] [SERVICE...]

        Options:
            -f, --force   Don't ask to confirm removal
            -v            Remove volumes associated with containers
        """
        all_containers = project.containers(service_names=options['SERVICE'], stopped=True)
        stopped_containers = [c for c in all_containers if not c.is_running]

        if len(stopped_containers) > 0:
            print("Going to remove", list_containers(stopped_containers))
            if options.get('--force') \
                    or yesno("Are you sure? [yN] ", default=False):
                project.remove_stopped(
                    service_names=options['SERVICE'],
                    v=options.get('-v', False)
                )
        else:
            print("No stopped containers")

    def run(self, project, options):
        """
        Run a one-off command on a service.

        For example:

            $ docker-compose run web python manage.py shell

        By default, linked services will be started, unless they are already
        running. If you do not want to start linked services, use
        `docker-compose run --no-deps SERVICE COMMAND [ARGS...]`.

        Usage: run [options] [-p PORT...] [-e KEY=VAL...] SERVICE [COMMAND] [ARGS...]

        Options:
            --allow-insecure-ssl  Deprecated - no effect.
            -d                    Detached mode: Run container in the background, print
                                  new container name.
            --name NAME           Assign a name to the container
            --entrypoint CMD      Override the entrypoint of the image.
            -e KEY=VAL            Set an environment variable (can be used multiple times)
            -u, --user=""         Run as specified username or uid
            --no-deps             Don't start linked services.
            --rm                  Remove container after run. Ignored in detached mode.
            -p, --publish=[]      Publish a container's port(s) to the host
            --service-ports       Run command with the service's ports enabled and mapped
                                  to the host.
            -T                    Disable pseudo-tty allocation. By default `docker-compose run`
                                  allocates a TTY.
        """
        service = project.get_service(options['SERVICE'])

        if options['--allow-insecure-ssl']:
            log.warn(INSECURE_SSL_WARNING)

        if not options['--no-deps']:
            deps = service.get_linked_names()

            if len(deps) > 0:
                project.up(
                    service_names=deps,
                    start_deps=True,
                    allow_recreate=False,
                )

        tty = True
        if options['-d'] or options['-T'] or not sys.stdin.isatty():
            tty = False

        if options['COMMAND']:
            command = [options['COMMAND']] + options['ARGS']
        else:
            command = service.options.get('command')

        container_options = {
            'command': command,
            'tty': tty,
            'stdin_open': not options['-d'],
            'detach': options['-d'],
        }

        if options['-e']:
            container_options['environment'] = parse_environment(options['-e'])

        if options['--entrypoint']:
            container_options['entrypoint'] = options.get('--entrypoint')

        if options['--rm']:
            container_options['restart'] = None

        if options['--user']:
            container_options['user'] = options.get('--user')

        if not options['--service-ports']:
            container_options['ports'] = []

        if options['--publish']:
            container_options['ports'] = options.get('--publish')

        if options['--publish'] and options['--service-ports']:
            raise UserError(
                'Service port mapping and manual port mapping '
                'can not be used togather'
            )

        if options['--name']:
            container_options['name'] = options['--name']

        try:
            container = service.create_container(
                quiet=True,
                one_off=True,
                **container_options
            )
        except APIError as e:
            legacy.check_for_legacy_containers(
                project.client,
                project.name,
                [service.name],
                allow_one_off=False,
            )

            raise e

        if options['-d']:
            service.start_container(container)
            print(container.name)
        else:
            dockerpty.start(project.client, container.id, interactive=not options['-T'])
            exit_code = container.wait()
            if options['--rm']:
                project.client.remove_container(container.id)
            sys.exit(exit_code)

    def scale(self, project, options):
        """
        Set number of containers to run for a service.

        Numbers are specified in the form `service=num` as arguments.
        For example:

            $ docker-compose scale web=2 worker=3

        Usage: scale [options] [SERVICE=NUM...]

        Options:
          -t, --timeout TIMEOUT      Specify a shutdown timeout in seconds.
                                     (default: 10)
        """
        timeout = int(options.get('--timeout') or DEFAULT_TIMEOUT)

        for s in options['SERVICE=NUM']:
            if '=' not in s:
                raise UserError('Arguments to scale should be in the form service=num')
            service_name, num = s.split('=', 1)
            try:
                num = int(num)
            except ValueError:
                raise UserError('Number of containers for service "%s" is not a '
                                'number' % service_name)
            project.get_service(service_name).scale(num, timeout=timeout)

    def start(self, project, options):
        """
        Start existing containers.

        Usage: start [SERVICE...]
        """
        project.start(service_names=options['SERVICE'])

    def stop(self, project, options):
        """
        Stop running containers without removing them.

        They can be started again with `docker-compose start`.

        Usage: stop [options] [SERVICE...]

        Options:
          -t, --timeout TIMEOUT      Specify a shutdown timeout in seconds.
                                     (default: 10)
        """
        timeout = int(options.get('--timeout') or DEFAULT_TIMEOUT)
        project.stop(service_names=options['SERVICE'], timeout=timeout)

    def restart(self, project, options):
        """
        Restart running containers.

        Usage: restart [options] [SERVICE...]

        Options:
          -t, --timeout TIMEOUT      Specify a shutdown timeout in seconds.
                                     (default: 10)
        """
        timeout = int(options.get('--timeout') or DEFAULT_TIMEOUT)
        project.restart(service_names=options['SERVICE'], timeout=timeout)

    def unpause(self, project, options):
        """
        Unpause services.

        Usage: unpause [SERVICE...]
        """
        project.unpause(service_names=options['SERVICE'])

    def up(self, project, options):
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

        Usage: up [options] [SERVICE...]

        Options:
            --allow-insecure-ssl   Deprecated - no effect.
            -d                     Detached mode: Run containers in the background,
                                   print new container names.
            --no-color             Produce monochrome output.
            --no-deps              Don't start linked services.
            --force-recreate       Recreate containers even if their configuration and
                                   image haven't changed. Incompatible with --no-recreate.
            --no-recreate          If containers already exist, don't recreate them.
                                   Incompatible with --force-recreate.
            --no-build             Don't build an image, even if it's missing
            -t, --timeout TIMEOUT  Use this timeout in seconds for container shutdown
                                   when attached or when containers are already
                                   running. (default: 10)
        """
        if options['--allow-insecure-ssl']:
            log.warn(INSECURE_SSL_WARNING)

        detached = options['-d']

        monochrome = options['--no-color']

        start_deps = not options['--no-deps']
        allow_recreate = not options['--no-recreate']
        force_recreate = options['--force-recreate']
        service_names = options['SERVICE']
        timeout = int(options.get('--timeout') or DEFAULT_TIMEOUT)

        if force_recreate and not allow_recreate:
            raise UserError("--force-recreate and --no-recreate cannot be combined.")

        to_attach = project.up(
            service_names=service_names,
            start_deps=start_deps,
            allow_recreate=allow_recreate,
            force_recreate=force_recreate,
            do_build=not options['--no-build'],
            timeout=timeout
        )

        if not detached:
            log_printer = build_log_printer(to_attach, service_names, monochrome)
            attach_to_logs(project, log_printer, service_names, timeout)

    def migrate_to_labels(self, project, _options):
        """
        Recreate containers to add labels

        If you're coming from Compose 1.2 or earlier, you'll need to remove or
        migrate your existing containers after upgrading Compose. This is
        because, as of version 1.3, Compose uses Docker labels to keep track
        of containers, and so they need to be recreated with labels added.

        If Compose detects containers that were created without labels, it
        will refuse to run so that you don't end up with two sets of them. If
        you want to keep using your existing containers (for example, because
        they have data volumes you want to preserve) you can migrate them with
        the following command:

            docker-compose migrate-to-labels

        Alternatively, if you're not worried about keeping them, you can
        remove them - Compose will just create new ones.

            docker rm -f myapp_web_1 myapp_db_1 ...

        Usage: migrate-to-labels
        """
        legacy.migrate_project_to_labels(project)

    def version(self, project, options):
        """
        Show version informations

        Usage: version [--short]

        Options:
            --short     Shows only Compose's version number.
        """
        if options['--short']:
            print(__version__)
        else:
            print(get_version_info('full'))


def build_log_printer(containers, service_names, monochrome):
    return LogPrinter(
        [c for c in containers if c.service in service_names],
        attach_params={"logs": True},
        monochrome=monochrome)


def attach_to_logs(project, log_printer, service_names, timeout):
    print("Attaching to", list_containers(log_printer.containers))
    try:
        log_printer.run()
    finally:
        def handler(signal, frame):
            project.kill(service_names=service_names)
            sys.exit(0)
        signal.signal(signal.SIGINT, handler)

        print("Gracefully stopping... (press Ctrl+C again to force)")
        project.stop(service_names=service_names, timeout=timeout)


def list_containers(containers):
    return ", ".join(c.name for c in containers)
