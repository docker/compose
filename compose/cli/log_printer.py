from __future__ import unicode_literals
from __future__ import absolute_import
import sys

from itertools import cycle

from .multiplexer import Multiplexer, STOP
from . import colors
from .utils import split_buffer
from requests.exceptions import ConnectionError


class LogPrinter(object):
    def __init__(self, containers, attach_params=None, output=sys.stdout, monochrome=False):
        self.containers = containers
        self.attach_params = attach_params or {}
        self.prefix_width = self._calculate_prefix_width(containers)
        self.generators = self._make_log_generators(monochrome)
        self.output = output

    def run(self):
        mux = Multiplexer(self.generators)
        for line in mux.loop():
            self.output.write(line)

    def _calculate_prefix_width(self, containers):
        """
        Calculate the maximum width of container names so we can make the log
        prefixes line up like so:

        db_1  | Listening
        web_1 | Listening
        """
        prefix_width = 0
        for container in containers:
            prefix_width = max(prefix_width, len(container.name_without_project))
        return prefix_width

    def _make_log_generators(self, monochrome):
        color_fns = cycle(colors.rainbow())
        generators = []

        def no_color(text):
            return text

        for container in self.containers:
            if monochrome:
                color_fn = no_color
            else:
                color_fn = next(color_fns)
            generators.append(self._make_log_generator(container, color_fn))

        return generators

    def _make_log_generator(self, container, color_fn):
        try:
            prefix = color_fn(self._generate_prefix(container)).encode('utf-8')
            # Attach to container before log printer starts running
            line_generator = split_buffer(self._attach(container), '\n')

            for line in line_generator:
                yield prefix + line

            exit_code = container.wait()
            yield color_fn("%s exited with code %s\n" % (container.name, exit_code))
            yield STOP
        except ConnectionError:
            yield color_fn("%s: Unable to connect to Docker Api. Aborting.\n" % (container.name))
            yield STOP

    def _generate_prefix(self, container):
        """
        Generate the prefix for a log line without colour
        """
        name = container.name_without_project
        padding = ' ' * (self.prefix_width - len(name))
        return ''.join([name, padding, ' | '])

    def _attach(self, container):
        params = {
            'stdout': True,
            'stderr': True,
            'stream': True,
        }
        params.update(self.attach_params)
        params = dict((name, 1 if value else 0) for (name, value) in list(params.items()))
        return container.attach(**params)
