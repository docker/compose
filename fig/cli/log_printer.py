from __future__ import unicode_literals
from __future__ import absolute_import
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
        prefix = color_fn(container.name + " | ")
        for line in split_buffer(self._attach(container), '\n'):
            yield prefix + line

    def _attach(self, container):
        params = {
            'stdout': True,
            'stderr': True,
            'stream': True,
        }
        params.update(self.attach_params)
        params = dict((name, 1 if value else 0) for (name, value) in list(params.items()))
        return container.attach(**params)

def split_buffer(reader, separator):
    """
    Given a generator which yields strings and a separator string,
    joins all input, splits on the separator and yields each chunk.
    Requires that each input string is decodable as UTF-8.
    """
    buffered = ''

    for data in reader:
        lines = (buffered + data.decode('utf-8')).split(separator)
        for line in lines[:-1]:
            yield line + separator
        if len(lines) > 1:
            buffered = lines[-1]

    if len(buffered) > 0:
        yield buffered
