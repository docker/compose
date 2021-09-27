from inspect import getdoc

from docopt import docopt
from docopt import DocoptExit


def docopt_full_help(docstring, *args, **kwargs):
    try:
        return docopt(docstring, *args, **kwargs)
    except DocoptExit:
        raise SystemExit(docstring)


class DocoptDispatcher:

    def __init__(self, command_class, options):
        self.command_class = command_class
        self.options = options

    @classmethod
    def get_command_and_options(cls, doc_entity, argv, options):
        command_help = getdoc(doc_entity)
        opt = docopt_full_help(command_help, argv, **options)
        command = opt['COMMAND']
        return command_help, opt, command

    def parse(self, argv):
        command_help, options, command = DocoptDispatcher.get_command_and_options(
            self.command_class, argv, self.options)

        if command is None:
            raise SystemExit(command_help)

        handler = get_handler(self.command_class, command)
        docstring = getdoc(handler)

        if docstring is None:
            raise NoSuchCommand(command, self)

        command_options = docopt_full_help(docstring, options['ARGS'], options_first=True)
        return options, handler, command_options


def get_handler(command_class, command):
    command = command.replace('-', '_')
    # we certainly want to have "exec" command, since that's what docker client has
    # but in python exec is a keyword
    if command == "exec":
        command = "exec_command"

    if not hasattr(command_class, command):
        raise NoSuchCommand(command, command_class)

    return getattr(command_class, command)


class NoSuchCommand(Exception):
    def __init__(self, command, supercommand):
        super().__init__("No such command: %s" % command)

        self.command = command
        self.supercommand = supercommand
