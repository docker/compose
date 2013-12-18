import sys

from itertools import cycle

from .multiplexer import Multiplexer
from . import colors


class LogPrinter(object):
    def __init__(self, containers, attach_params=None):
        self.containers = containers
        self.attach_params = attach_params or {}
        self.generators = self._make_log_generators()

    def run(self):
        mux = Multiplexer(self.generators)
        for line in mux.loop():
            sys.stdout.write(line)

    def _make_log_generators(self):
        color_fns = cycle(colors.rainbow())
        generators = []

        for container in self.containers:
            color_fn = color_fns.next()
            generators.append(self._make_log_generator(container, color_fn))

        return generators

    def _make_log_generator(self, container, color_fn):
        format = lambda line: color_fn(container.name + " | ") + line
        return (format(line) for line in self._readlines(self._attach(container)))

    def _attach(self, container):
        params = {
            'stdin': False,
            'stdout': True,
            'stderr': True,
            'logs': False,
            'stream': True,
        }
        params.update(self.attach_params)
        params = dict((name, 1 if value else 0) for (name, value) in params.items())
        return container.attach_socket(params=params)

    def _readlines(self, socket):
        for line in iter(socket.makefile().readline, b''):
            if not line.endswith('\n'):
                line += '\n'

            yield line

        socket.close()
