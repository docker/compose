import sys

from inspect import getdoc
from docopt import docopt, DocoptExit


def docopt_full_help(docstring, *args, **kwargs):
    try:
        return docopt(docstring, *args, **kwargs)
    except DocoptExit:
        raise SystemExit(docstring)


class DocoptCommand(object):
    def sys_dispatch(self):
        self.dispatch(sys.argv[1:], None)

    def dispatch(self, argv, global_options):
        self.perform_command(*self.parse(argv, global_options))

    def perform_command(self, options, command, handler, command_options):
        handler(command_options)

    def parse(self, argv, global_options):
        options = docopt_full_help(getdoc(self), argv, options_first=True)
        command = options['COMMAND']

        if not hasattr(self, command):
            raise NoSuchCommand(command, self)

        handler = getattr(self, command)
        docstring = getdoc(handler)

        if docstring is None:
            raise NoSuchCommand(command, self)

        command_options = docopt_full_help(docstring, options['ARGS'], options_first=True)
        return (options, command, handler, command_options)


class NoSuchCommand(Exception):
    def __init__(self, command, supercommand):
        super(NoSuchCommand, self).__init__("No such command: %s" % command)

        self.command = command
        self.supercommand = supercommand
