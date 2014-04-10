from __future__ import unicode_literals
from __future__ import absolute_import
import sys

from itertools import cycle

from .multiplexer import Multiplexer, STOP
from . import colors
from .utils import split_buffer


class LogPrinter(object):
    def __init__(self, containers, attach_params=None):
        self.containers = containers
        self.attach_params = attach_params or {}
        self.generators = self._make_log_generators()

    def run(self):
        mux = Multiplexer(self.generators)
        for line in mux.loop():
            sys.stdout.write(line.encode(sys.__stdout__.encoding or 'utf-8'))

    def _make_log_generators(self):
        color_fns = cycle(colors.rainbow())
        generators = []

        for container in self.containers:
            color_fn = color_fns.next()
            generators.append(self._make_log_generator(container, color_fn))

        return generators

    def _make_log_generator(self, container, color_fn):
        prefix = color_fn(container.name + " | ")
        # Attach to container before log printer starts running
        line_generator = split_buffer(self._attach(container), '\n')

        for line in line_generator:
            yield prefix + line.decode('utf-8')

        exit_code = container.wait()
        yield color_fn("%s exited with code %s\n" % (container.name, exit_code))
        yield STOP

    def _attach(self, container):
        params = {
            'stdout': True,
            'stderr': True,
            'stream': True,
        }
        params.update(self.attach_params)
        params = dict((name, 1 if value else 0) for (name, value) in list(params.items()))
        return container.attach(**params)
