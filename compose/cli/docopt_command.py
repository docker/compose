from __future__ import absolute_import
from __future__ import unicode_literals

import sys
from inspect import getdoc

from docopt import docopt
from docopt import DocoptExit


def docopt_full_help(docstring, *args, **kwargs):
    try:
        return docopt(docstring, *args, **kwargs)
    except DocoptExit:
        raise SystemExit(docstring)


class DocoptCommand(object):
    def docopt_options(self):
        return {'options_first': True}

    def sys_dispatch(self):
        self.dispatch(sys.argv[1:])

    def dispatch(self, argv):
        self.perform_command(*self.parse(argv))

    def parse(self, argv):
        options = docopt_full_help(getdoc(self), argv, **self.docopt_options())
        command = options['COMMAND']

        if command is None:
            raise SystemExit(getdoc(self))

        handler = self.get_handler(command)
        docstring = getdoc(handler)

        if docstring is None:
            raise NoSuchCommand(command, self)

        command_options = docopt_full_help(docstring, options['ARGS'], options_first=True)
        return options, handler, command_options

    def get_handler(self, command):
        command = command.replace('-', '_')
        # we certainly want to have "exec" command, since that's what docker client has
        # but in python exec is a keyword
        if command == "exec":
            command = "exec_command"

        if not hasattr(self, command):
            raise NoSuchCommand(command, self)

        return getattr(self, command)


class NoSuchCommand(Exception):
    def __init__(self, command, supercommand):
        super(NoSuchCommand, self).__init__("No such command: %s" % command)

        self.command = command
        self.supercommand = supercommand
