import sys

from itertools import cycle

from ..service import get_container_name
from .multiplexer import Multiplexer
from . import colors


class LogPrinter(object):
    def __init__(self, client):
        self.client = client

    def attach(self, containers):
        generators = self._make_log_generators(containers)
        mux = Multiplexer(generators)
        for line in mux.loop():
            sys.stdout.write(line)

    def _make_log_generators(self, containers):
        color_fns = cycle(colors.rainbow())
        generators = []

        for container in containers:
            color_fn = color_fns.next()
            generators.append(self._make_log_generator(container, color_fn))

        return generators

    def _make_log_generator(self, container, color_fn):
        container_name = get_container_name(container)
        format = lambda line: color_fn(container_name + " | ") + line
        return (format(line) for line in self._readlines(container))

    def _readlines(self, container, logs=False, stream=True):
        socket = self.client.attach_socket(
            container['Id'],
            params={
                'stdin': 0,
                'stdout': 1,
                'stderr': 1,
                'logs': 1 if logs else 0,
                'stream': 1 if stream else 0
            },
        )

        for line in iter(socket.makefile().readline, b''):
            if not line.endswith('\n'):
                line += '\n'

            yield line

        socket.close()
