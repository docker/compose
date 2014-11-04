from __future__ import print_function
from __future__ import unicode_literals
import logging
import sys
import re
import signal
from operator import attrgetter

from inspect import getdoc
import dockerpty

from .. import __version__
from ..project import NoSuchService, ConfigurationError
from ..service import BuildError, CannotBeScaledError
from .command import Command
from .formatter import Formatter
from .log_printer import LogPrinter
from .utils import yesno

from docker.errors import APIError
from .errors import UserError
from .docopt_command import NoSuchCommand

log = logging.getLogger(__name__)


def main():
    setup_logging()
    try:
        command = TopLevelCommand()
        command.sys_dispatch()
    except KeyboardInterrupt:
        log.error("\nAborting.")
        sys.exit(1)
    except (UserError, NoSuchService, ConfigurationError) as e:
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


def setup_logging():
    console_handler = logging.StreamHandler(sys.stderr)
    console_handler.setFormatter(logging.Formatter())
    console_handler.setLevel(logging.INFO)
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
    """Fast, isolated development environments using Docker.

    Usage:
      fig [options] [COMMAND] [ARGS...]
      fig -h|--help

    Options:
      --verbose                 Show more output
      --version                 Print version and exit
      -f, --file FILE           Specify an alternate fig file (default: fig.yml)
      -p, --project-name NAME   Specify an alternate project name (default: directory name)

    Commands:
      build     Build or rebuild services
      help      Get help on a command
      kill      Kill containers
      logs      View output from containers
      port      Print the public port for a port binding
      ps        List containers
      pull      Pulls service images
      rm        Remove stopped containers
      run       Run a one-off command
      scale     Set number of containers for a service
      start     Start services
      stop      Stop services
      restart   Restart services
      up        Create and start containers

    """
    def docopt_options(self):
        options = super(TopLevelCommand, self).docopt_options()
        options['version'] = "fig %s" % __version__
        return options

    def build(self, project, options):
        """
        Build or rebuild services.

        Services are built once and then tagged as `project_service`,
        e.g. `figtest_db`. If you change a service's `Dockerfile` or the
        contents of its build directory, you can run `fig build` to rebuild it.

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
        command = options['COMMAND']
        if not hasattr(self, command):
            raise NoSuchCommand(command, self)
        raise SystemExit(getdoc(getattr(self, command)))

    def kill(self, project, options):
        """
        Force stop service containers.

        Usage: kill [SERVICE...]
        """
        project.kill(service_names=options['SERVICE'])

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

    def port(self, project, options):
        """
        Print the public port for a port binding.

        Usage: port [options] SERVICE PRIVATE_PORT

        Options:
            --protocol=proto  tcp or udp (defaults to tcp)
            --index=index     index of the container if there are multiple
                              instances of a service (defaults to 1)
        """
        service = project.get_service(options['SERVICE'])
        try:
            container = service.get_container(number=options.get('--index') or 1)
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
            --allow-insecure-ssl    Allow insecure connections to the docker
                                    registry
        """
        insecure_registry = options['--allow-insecure-ssl']
        project.pull(
            service_names=options['SERVICE'],
            insecure_registry=insecure_registry
        )

    def rm(self, project, options):
        """
        Remove stopped service containers.

        Usage: rm [options] [SERVICE...]

        Options:
            --force   Don't ask to confirm removal
            -v        Remove volumes associated with containers
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

            $ fig run web python manage.py shell

        By default, linked services will be started, unless they are already
        running. If you do not want to start linked services, use
        `fig run --no-deps SERVICE COMMAND [ARGS...]`.

        Usage: run [options] [-e KEY=VAL...] SERVICE [COMMAND] [ARGS...]

        Options:
            --allow-insecure-ssl  Allow insecure connections to the docker
                                  registry
            -d                    Detached mode: Run container in the background, print
                                  new container name.
            --entrypoint CMD      Override the entrypoint of the image.
            -e KEY=VAL            Set an environment variable (can be used multiple times)
            --no-deps             Don't start linked services.
            --rm                  Remove container after run. Ignored in detached mode.
            -T                    Disable pseudo-tty allocation. By default `fig run`
                                  allocates a TTY.
        """
        service = project.get_service(options['SERVICE'])

        insecure_registry = options['--allow-insecure-ssl']

        if not options['--no-deps']:
            deps = service.get_linked_names()

            if len(deps) > 0:
                project.up(
                    service_names=deps,
                    start_links=True,
                    recreate=False,
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
        }

        if options['-e']:
            for option in options['-e']:
                if 'environment' not in service.options:
                    service.options['environment'] = {}
                k, v = option.split('=', 1)
                service.options['environment'][k] = v

        if options['--entrypoint']:
            container_options['entrypoint'] = options.get('--entrypoint')
        container = service.create_container(
            one_off=True,
            insecure_registry=insecure_registry,
            **container_options
        )
        if options['-d']:
            service.start_container(container, ports=None, one_off=True)
            print(container.name)
        else:
            service.start_container(container, ports=None, one_off=True)
            dockerpty.start(project.client, container.id, interactive=not options['-T'])
            exit_code = container.wait()
            if options['--rm']:
                log.info("Removing %s..." % container.name)
                project.client.remove_container(container.id)
            sys.exit(exit_code)

    def scale(self, project, options):
        """
        Set number of containers to run for a service.

        Numbers are specified in the form `service=num` as arguments.
        For example:

            $ fig scale web=2 worker=3

        Usage: scale [SERVICE=NUM...]
        """
        for s in options['SERVICE=NUM']:
            if '=' not in s:
                raise UserError('Arguments to scale should be in the form service=num')
            service_name, num = s.split('=', 1)
            try:
                num = int(num)
            except ValueError:
                raise UserError('Number of containers for service "%s" is not a '
                                'number' % service_name)
            try:
                project.get_service(service_name).scale(num)
            except CannotBeScaledError:
                raise UserError(
                    'Service "%s" cannot be scaled because it specifies a port '
                    'on the host. If multiple containers for this service were '
                    'created, the port would clash.\n\nRemove the ":" from the '
                    'port definition in fig.yml so Docker can choose a random '
                    'port for each container.' % service_name)

    def start(self, project, options):
        """
        Start existing containers.

        Usage: start [SERVICE...]
        """
        project.start(service_names=options['SERVICE'])

    def stop(self, project, options):
        """
        Stop running containers without removing them.

        They can be started again with `fig start`.

        Usage: stop [SERVICE...]
        """
        project.stop(service_names=options['SERVICE'])

    def restart(self, project, options):
        """
        Restart running containers.

        Usage: restart [SERVICE...]
        """
        project.restart(service_names=options['SERVICE'])

    def up(self, project, options):
        """
        Build, (re)create, start and attach to containers for a service.

        By default, `fig up` will aggregate the output of each container, and
        when it exits, all containers will be stopped. If you run `fig up -d`,
        it'll start the containers in the background and leave them running.

        If there are existing containers for a service, `fig up` will stop
        and recreate them (preserving mounted volumes with volumes-from),
        so that changes in `fig.yml` are picked up. If you do not want existing
        containers to be recreated, `fig up --no-recreate` will re-use existing
        containers.

        Usage: up [options] [SERVICE...]

        Options:
            --allow-insecure-ssl  Allow insecure connections to the docker
                                  registry
            -d                    Detached mode: Run containers in the background,
                                  print new container names.
            --no-color            Produce monochrome output.
            --no-deps             Don't start linked services.
            --no-recreate         If containers already exist, don't recreate them.
        """
        insecure_registry = options['--allow-insecure-ssl']
        detached = options['-d']

        monochrome = options['--no-color']

        start_links = not options['--no-deps']
        recreate = not options['--no-recreate']
        service_names = options['SERVICE']

        project.up(
            service_names=service_names,
            start_links=start_links,
            recreate=recreate,
            insecure_registry=insecure_registry,
        )

        to_attach = [c for s in project.get_services(service_names) for c in s.containers()]

        if not detached:
            print("Attaching to", list_containers(to_attach))
            log_printer = LogPrinter(to_attach, attach_params={"logs": True}, monochrome=monochrome)

            try:
                log_printer.run()
            finally:
                def handler(signal, frame):
                    project.kill(service_names=service_names)
                    sys.exit(0)
                signal.signal(signal.SIGINT, handler)

                print("Gracefully stopping... (press Ctrl+C again to force)")
                project.stop(service_names=service_names)


def list_containers(containers):
    return ", ".join(c.name for c in containers)
