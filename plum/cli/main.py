import logging
import sys
import re

from inspect import getdoc

from .. import __version__
from ..project import NoSuchService
from .command import Command
from .formatter import Formatter
from .log_printer import LogPrinter

from docker.client import APIError
from .errors import UserError
from .docopt_command import NoSuchCommand
from .socketclient import SocketClient

log = logging.getLogger(__name__)


def main():
    console_handler = logging.StreamHandler()
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
        exit(1)
    except UserError, e:
        log.error(e.msg)
        exit(1)
    except NoSuchService, e:
        log.error(e.msg)
        exit(1)
    except NoSuchCommand, e:
        log.error("No such command: %s", e.command)
        log.error("")
        log.error("\n".join(parse_doc_section("commands:", getdoc(e.supercommand))))
        exit(1)
    except APIError, e:
        log.error(e.explanation)
        exit(1)


# stolen from docopt master
def parse_doc_section(name, source):
    pattern = re.compile('^([^\n]*' + name + '[^\n]*\n?(?:[ \t].*?(?:\n|$))*)',
                         re.IGNORECASE | re.MULTILINE)
    return [s.strip() for s in pattern.findall(source)]


class TopLevelCommand(Command):
    """.

    Usage:
      plum [options] [COMMAND] [ARGS...]
      plum -h|--help

    Options:
      --verbose            Show more output
      --version            Print version and exit

    Commands:
      logs      View output from containers
      ps        List services and containers
      run       Run a one-off command
      start     Start services
      stop      Stop services
      kill      Kill containers
      rm        Remove stopped containers

    """
    def docopt_options(self):
        options = super(TopLevelCommand, self).docopt_options()
        options['version'] = "plum %s" % __version__
        return options

    def ps(self, options):
        """
        List services and containers.

        Usage: ps [options]

        Options:
            -q    Only display IDs
        """
        containers = self.project.containers(stopped=True) + self.project.containers(one_off=True)

        if options['-q']:
            for container in containers:
                print container.id
        else:
            headers = [
                'Name',
                'Command',
                'State',
                'Ports',
            ]
            rows = []
            for container in containers:
                rows.append([
                    container.name,
                    container.human_readable_command,
                    container.human_readable_state,
                    container.human_readable_ports,
                ])
            print Formatter().table(headers, rows)

    def run(self, options):
        """
        Run a one-off command.

        Usage: run [options] SERVICE COMMAND [ARGS...]

        Options:
            -d    Detached mode: Run container in the background, print new container name
        """
        service = self.project.get_service(options['SERVICE'])
        container_options = {
            'command': [options['COMMAND']] + options['ARGS'],
            'tty': not options['-d'],
            'stdin_open': not options['-d'],
        }
        container = service.create_container(one_off=True, **container_options)
        if options['-d']:
            service.start_container(container, ports=None)
            print container.name
        else:
            with self._attach_to_container(
                container.id,
                interactive=True,
                logs=True,
                raw=True
            ) as c:
                service.start_container(container, ports=None)
                c.run()

    def up(self, options):
        """
        Create and start containers

        Usage: up [options] [SERVICE...]

        Options:
            -d    Detached mode: Run containers in the background, print new container names
        """
        detached = options['-d']

        unstarted = self.project.create_containers(service_names=options['SERVICE'])

        if not detached:
            to_attach = self.project.containers(service_names=options['SERVICE']) + [c for (s, c) in unstarted]
            print "Attaching to", list_containers(to_attach)
            log_printer = LogPrinter(to_attach, attach_params={'logs': True})

        for (s, c) in unstarted:
            s.start_container(c)

        if detached:
            for (s, c) in unstarted:
                print c.name
        else:
            try:
                log_printer.run()
            finally:
                self.project.kill_and_remove(unstarted)

    def start(self, options):
        """
        Start all services

        Usage: start [SERVICE...]
        """
        self.project.start(service_names=options['SERVICE'])

    def stop(self, options):
        """
        Stop all services

        Usage: stop [SERVICE...]
        """
        self.project.stop(service_names=options['SERVICE'])

    def kill(self, options):
        """
        Kill all containers

        Usage: kill [SERVICE...]
        """
        self.project.kill(service_names=options['SERVICE'])

    def rm(self, options):
        """
        Remove all stopped containers

        Usage: rm [SERVICE...]
        """
        self.project.remove_stopped(service_names=options['SERVICE'])

    def logs(self, options):
        """
        View output from containers

        Usage: logs
        """
        containers = self.project.containers(stopped=False)
        print "Attaching to", list_containers(containers)
        LogPrinter(containers, attach_params={'logs': True}).run()

    def _attach_to_container(self, container_id, interactive, logs=False, stream=True, raw=False):
        stdio = self.client.attach_socket(
            container_id,
            params={
                'stdin': 1 if interactive else 0,
                'stdout': 1,
                'stderr': 0,
                'logs': 1 if logs else 0,
                'stream': 1 if stream else 0
            },
            ws=True,
        )

        stderr = self.client.attach_socket(
            container_id,
            params={
                'stdin': 0,
                'stdout': 0,
                'stderr': 1,
                'logs': 1 if logs else 0,
                'stream': 1 if stream else 0
            },
            ws=True,
        )

        return SocketClient(
            socket_in=stdio,
            socket_out=stdio,
            socket_err=stderr,
            raw=raw,
        )

def list_containers(containers):
    return ", ".join(c.name for c in containers)
