from __future__ import absolute_import
from __future__ import unicode_literals

import sys
from itertools import cycle

from six import next

from . import colors
from .multiplexer import Multiplexer
from .utils import split_buffer
from compose import utils


class LogPrinter(object):
    # TODO: move logic to run
    def __init__(self, containers, output=sys.stdout, monochrome=False):
        self.containers = containers
        self.prefix_width = self._calculate_prefix_width(containers)
        self.generators = self._make_log_generators(monochrome)
        self.output = utils.get_output_stream(output)

    def run(self):
        mux = Multiplexer(self.generators)
        for line in mux.loop():
            self.output.write(line)

    # TODO: doesn't use self, remove from class
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
        prefix = color_fn(self._generate_prefix(container))

        if container.has_api_logs:
            return build_log_generator(container, prefix, color_fn)
        return build_no_log_generator(container, prefix, color_fn)

    def _generate_prefix(self, container):
        """
        Generate the prefix for a log line without colour
        """
        name = container.name_without_project
        padding = ' ' * (self.prefix_width - len(name))
        return ''.join([name, padding, ' | '])


def build_no_log_generator(container, prefix, color_fn):
    """Return a generator that prints a warning about logs and waits for
    container to exit.
    """
    yield "{} WARNING: no logs are available with the '{}' log driver\n".format(
        prefix,
        container.log_driver)
    yield color_fn(wait_on_exit(container))


def build_log_generator(container, prefix, color_fn):
    # Attach to container before log printer starts running
    stream = container.attach(stdout=True, stderr=True,  stream=True, logs=True)
    line_generator = split_buffer(stream, u'\n')

    for line in line_generator:
        yield prefix + line
    yield color_fn(wait_on_exit(container))


def wait_on_exit(container):
    exit_code = container.wait()
    return "%s exited with code %s\n" % (container.name, exit_code)
