import datetime
import logging
import sys
import os
import re

from docopt import docopt
from inspect import getdoc

from .. import __version__
from ..service_collection import ServiceCollection
from .command import Command

from .errors import UserError
from .docopt_command import NoSuchCommand

log = logging.getLogger(__name__)

def main():
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

    """
    def ps(self, options):
        """
        List services and containers.

        Usage: ps
        """
        for service in self.service_collection:
            for container in service.containers:
                print container['Names'][0]

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

