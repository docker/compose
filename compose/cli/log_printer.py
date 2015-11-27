from __future__ import absolute_import
from __future__ import unicode_literals

import sys
from itertools import cycle

from . import colors
from .multiplexer import Multiplexer
from compose import utils
from compose.utils import split_buffer


class LogPrinter(object):
    """Print logs from many containers to a single output stream."""

    def __init__(self, containers, output=sys.stdout, monochrome=False):
        self.containers = containers
        self.output = utils.get_output_stream(output)
        self.monochrome = monochrome

    def run(self):
        if not self.containers:
            return

        prefix_width = max_name_width(self.containers)
        generators = list(self._make_log_generators(self.monochrome, prefix_width))
        for line in Multiplexer(generators).loop():
            self.output.write(line)
            self.output.flush()

    def _make_log_generators(self, monochrome, prefix_width):
        def no_color(text):
            return text

        if monochrome:
            color_funcs = cycle([no_color])
        else:
            color_funcs = cycle(colors.rainbow())

        for color_func, container in zip(color_funcs, self.containers):
            generator_func = get_log_generator(container)
            prefix = color_func(build_log_prefix(container, prefix_width))
            yield generator_func(container, prefix, color_func)


def build_log_prefix(container, prefix_width):
    return container.name_without_project.ljust(prefix_width) + ' | '


def max_name_width(containers):
    """Calculate the maximum width of container names so we can make the log
    prefixes line up like so:

    db_1  | Listening
    web_1 | Listening
    """
    return max(len(container.name_without_project) for container in containers)


def get_log_generator(container):
    if container.has_api_logs:
        return build_log_generator
    return build_no_log_generator


def build_no_log_generator(container, prefix, color_func):
    """Return a generator that prints a warning about logs and waits for
    container to exit.
    """
    yield "{} WARNING: no logs are available with the '{}' log driver\n".format(
        prefix,
        container.log_driver)
    yield color_func(wait_on_exit(container))


def build_log_generator(container, prefix, color_func):
    # if the container doesn't have a log_stream we need to attach to container
    # before log printer starts running
    if container.log_stream is None:
        stream = container.attach(stdout=True, stderr=True,  stream=True, logs=True)
        line_generator = split_buffer(stream)
    else:
        line_generator = split_buffer(container.log_stream)

    for line in line_generator:
        yield prefix + line
    yield color_func(wait_on_exit(container))


def wait_on_exit(container):
    exit_code = container.wait()
    return "%s exited with code %s\n" % (container.name, exit_code)
