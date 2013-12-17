import datetime
import logging
import sys
import os
import re

from docopt import docopt
from inspect import getdoc

from .. import __version__
from ..service import get_container_name
from ..service_collection import ServiceCollection
from .command import Command

from .errors import UserError
from .docopt_command import NoSuchCommand

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
    except NoSuchCommand, e:
        log.error("No such command: %s", e.command)
        log.error("")
        log.error("\n".join(parse_doc_section("commands:", getdoc(e.supercommand))))
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
      ps        List services and containers
      run       Run a one-off command
      start     Start services
      stop      Stop services

    """
    def ps(self, options):
        """
        List services and containers.

        Usage: ps
        """
        for service in self.service_collection:
            for container in service.containers:
                print get_container_name(container)

    def run(self, options):
        """
        Run a one-off command.

        Usage: run SERVICE COMMAND [ARGS...]
        """
        service = self.service_collection.get(options['SERVICE'])
        container_options = {
            'command': [options['COMMAND']] + options['ARGS'],
        }
        container = service.create_container(**container_options)
        stream = self.client.logs(container, stream=True)
        service.start_container(container, **container_options)
        for data in stream:
            if data is None:
                break
            print data

    def start(self, options):
        """
        Start all services

        Usage: start
        """
        self.service_collection.start()

    def stop(self, options):
        """
        Stop all services

        Usage: stop
        """
        self.service_collection.stop()

