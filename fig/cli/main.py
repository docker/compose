from __future__ import print_function
from __future__ import unicode_literals
import logging
import sys
import re
import signal

from inspect import getdoc

from .. import __version__
from ..project import NoSuchService, ConfigurationError
from ..service import BuildError, CannotBeScaledError
from .command import Command
from .formatter import Formatter
from .log_printer import LogPrinter
from .utils import yesno
from .ttysizer import TTYSizer

from ..packages.docker.errors import APIError
from .errors import UserError
from .docopt_command import NoSuchCommand
from .socketclient import SocketClient

log = logging.getLogger(__name__)


def main():
    console_handler = logging.StreamHandler(stream=sys.stderr)
    console_handler.setFormatter(logging.Formatter())
    console_handler.setLevel(logging.INFO)
    root_logger = logging.getLogger()
    root_logger.addHandler(console_handler)
    root_logger.setLevel(logging.DEBUG)

    # Disable requests logging
    logging.getLogger("requests").propagate = False

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


# stolen from docopt master
def parse_doc_section(name, source):
    pattern = re.compile('^([^\n]*' + name + '[^\n]*\n?(?:[ \t].*?(?:\n|$))*)',
                         re.IGNORECASE | re.MULTILINE)
    return [s.strip() for s in pattern.findall(source)]


class TopLevelCommand(Command):
    """Punctual, lightweight development environments using Docker.

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
      ps        List containers
      rm        Remove stopped containers
      run       Run a one-off command
      scale     Set number of containers for a service
      start     Start services
      stop      Stop services
      up        Create and start containers

    """
    def docopt_options(self):
        options = super(TopLevelCommand, self).docopt_options()
        options['version'] = "fig %s" % __version__
        return options

    def build(self, options):
        """
        Build or rebuild services.

        Services are built once and then tagged as `project_service`,
        e.g. `figtest_db`. If you change a service's `Dockerfile` or the
        contents of its build directory, you can run `fig build` to rebuild it.

        Usage: build [SERVICE...]
        """
        self.project.build(service_names=options['SERVICE'])

    def help(self, options):
        """
        Get help on a command.

        Usage: help COMMAND
        """
        command = options['COMMAND']
        if not hasattr(self, command):
            raise NoSuchCommand(command, self)
        raise SystemExit(getdoc(getattr(self, command)))

    def kill(self, options):
        """
        Force stop service containers.

        Usage: kill [SERVICE...]
        """
        self.project.kill(service_names=options['SERVICE'])

    def logs(self, options):
        """
        View output from containers.

        Usage: logs [SERVICE...]
        """
        containers = self.project.containers(service_names=options['SERVICE'], stopped=True)
        print("Attaching to", list_containers(containers))
        LogPrinter(containers, attach_params={'logs': True}).run()

    def ps(self, options):
        """
        List containers.

        Usage: ps [options] [SERVICE...]

        Options:
            -q    Only display IDs
        """
        containers = self.project.containers(service_names=options['SERVICE'], stopped=True) + self.project.containers(service_names=options['SERVICE'], one_off=True)

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

    def rm(self, options):
        """
        Remove stopped service containers.

        Usage: rm [options] [SERVICE...]

        Options:
            --force   Don't ask to confirm removal
            -v        Remove volumes associated with containers
        """
        all_containers = self.project.containers(service_names=options['SERVICE'], stopped=True)
        stopped_containers = [c for c in all_containers if not c.is_running]

        if len(stopped_containers) > 0:
            print("Going to remove", list_containers(stopped_containers))
            if options.get('--force') \
                    or yesno("Are you sure? [yN] ", default=False):
                self.project.remove_stopped(
                    service_names=options['SERVICE'],
                    v=options.get('-v', False)
                )
        else:
            print("No stopped containers")

    def run(self, options):
        """
        Run a one-off command on a service.

        For example:

            $ fig run web python manage.py shell

        By default, linked services will be started, unless they are already
        running. If you do not want to start linked services, use
        `fig run --no-deps SERVICE COMMAND [ARGS...]`.

        Usage: run [options] SERVICE COMMAND [ARGS...]

        Options:
            -d         Detached mode: Run container in the background, print
                       new container name.
            -T         Disable pseudo-tty allocation. By default `fig run`
                       allocates a TTY.
            --rm       Remove container after run. Ignored in detached mode.
            --no-deps  Don't start linked services.
        """

        service = self.project.get_service(options['SERVICE'])

        if not options['--no-deps']:
            self.project.up(
                service_names=service.get_linked_names(),
                start_links=True,
                recreate=False
            )

        tty = True
        if options['-d'] or options['-T'] or not sys.stdin.isatty():
            tty = False

        container_options = {
            'command': [options['COMMAND']] + options['ARGS'],
            'tty': tty,
            'stdin_open': not options['-d'],
        }
        container = service.create_container(one_off=True, **container_options)
        if options['-d']:
            service.start_container(container, ports=None, one_off=True)
            print(container.name)
        else:
            with self._attach_to_container(container, raw=tty) as c:
                service.start_container(container, ports=None, one_off=True)

                if tty:
                    tty_sizer = TTYSizer(container)
                    tty_sizer.start()

                c.run()

            exit_code = container.wait()
            if options['--rm']:
                log.info("Removing %s..." % container.name)
                self.client.remove_container(container.id)
            sys.exit(exit_code)

    def scale(self, options):
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
                raise UserError('Number of containers for service "%s" is not a number' % service)
            try:
                self.project.get_service(service_name).scale(num)
            except CannotBeScaledError:
                raise UserError('Service "%s" cannot be scaled because it specifies a port on the host. If multiple containers for this service were created, the port would clash.\n\nRemove the ":" from the port definition in fig.yml so Docker can choose a random port for each container.' % service_name)


    def start(self, options):
        """
        Start existing containers.

        Usage: start [SERVICE...]
        """
        self.project.start(service_names=options['SERVICE'])

    def stop(self, options):
        """
        Stop running containers without removing them.

        They can be started again with `fig start`.

        Usage: stop [SERVICE...]
        """
        self.project.stop(service_names=options['SERVICE'])

    def up(self, options):
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
            -d             Detached mode: Run containers in the background,
                           print new container names.
            --no-deps      Don't start linked services.
            --no-recreate  If containers already exist, don't recreate them.
        """
        detached = options['-d']

        start_links = not options['--no-deps']
        recreate = not options['--no-recreate']
        service_names = options['SERVICE']

        to_attach = self.project.up(
            service_names=service_names,
            start_links=start_links,
            recreate=recreate
        )

        if not detached:
            print("Attaching to", list_containers(to_attach))
            log_printer = LogPrinter(to_attach, attach_params={"logs": True})

            try:
                log_printer.run()
            finally:
                def handler(signal, frame):
                    self.project.kill(service_names=service_names)
                    sys.exit(0)
                signal.signal(signal.SIGINT, handler)

                print("Gracefully stopping... (press Ctrl+C again to force)")
                self.project.stop(service_names=service_names)

    def _attach_to_container(self, container, raw=False):
        socket_in = self.client.attach_socket(container.id, params={'stdin': 1, 'stream': 1})
        socket_out = self.client.attach_socket(container.id, params={'stdout': 1, 'logs': 1, 'stream': 1})
        socket_err = self.client.attach_socket(container.id, params={'stderr': 1, 'logs': 1, 'stream': 1})

        return SocketClient(
            socket_in=socket_in,
            socket_out=socket_out,
            socket_err=socket_err,
            raw=raw,
        )

def list_containers(containers):
    return ", ".join(c.name for c in containers)
