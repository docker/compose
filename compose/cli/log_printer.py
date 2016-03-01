from __future__ import absolute_import
from __future__ import unicode_literals

import sys
from itertools import cycle
from threading import Thread

from six.moves import _thread as thread
from six.moves.queue import Empty
from six.moves.queue import Queue

from . import colors
from compose import utils
from compose.cli.signals import ShutdownException
from compose.utils import split_buffer


STOP = object()


class LogPresenter(object):

    def __init__(self, prefix_width, color_func):
        self.prefix_width = prefix_width
        self.color_func = color_func

    def present(self, container, line):
        prefix = container.name_without_project.ljust(self.prefix_width)
        return '{prefix} {line}'.format(
            prefix=self.color_func(prefix + ' |'),
            line=line)


def build_log_presenters(service_names, monochrome):
    """Return an iterable of functions.

    Each function can be used to format the logs output of a container.
    """
    prefix_width = max_name_width(service_names)

    def no_color(text):
        return text

    for color_func in cycle([no_color] if monochrome else colors.rainbow()):
        yield LogPresenter(prefix_width, color_func)


def max_name_width(service_names, max_index_width=3):
    """Calculate the maximum width of container names so we can make the log
    prefixes line up like so:

    db_1  | Listening
    web_1 | Listening
    """
    return max(len(name) for name in service_names) + max_index_width


class LogPrinter(object):
    """Print logs from many containers to a single output stream."""

    def __init__(self,
                 containers,
                 presenters,
                 event_stream,
                 output=sys.stdout,
                 cascade_stop=False,
                 log_args=None):
        self.containers = containers
        self.presenters = presenters
        self.event_stream = event_stream
        self.output = utils.get_output_stream(output)
        self.cascade_stop = cascade_stop
        self.log_args = log_args or {}

    def run(self):
        if not self.containers:
            return

        queue = Queue()
        thread_args = queue, self.log_args
        thread_map = build_thread_map(self.containers, self.presenters, thread_args)
        start_producer_thread(
            thread_map,
            self.event_stream,
            self.presenters,
            thread_args)

        for line in consume_queue(queue, self.cascade_stop):
            self.output.write(line)
            self.output.flush()

            # TODO: this needs more logic
            # TODO: does consume_queue need to yield Nones to get to this point?
            if not thread_map:
                return


def build_thread_map(initial_containers, presenters, thread_args):
    def build_thread(container):
        tailer = Thread(
            target=tail_container_logs,
            args=(container, presenters.next()) + thread_args)
        tailer.daemon = True
        tailer.start()
        return tailer

    return {
        container.id: build_thread(container)
        for container in initial_containers
    }


def tail_container_logs(container, presenter, queue, log_args):
    generator = get_log_generator(container)

    try:
        for item in generator(container, log_args):
            queue.put((item, None))

        if log_args.get('follow'):
            yield presenter.color_func(wait_on_exit(container))

        queue.put((STOP, None))

    except Exception as e:
        queue.put((None, e))


def get_log_generator(container):
    if container.has_api_logs:
        return build_log_generator
    return build_no_log_generator


def build_no_log_generator(container, log_args):
    """Return a generator that prints a warning about logs and waits for
    container to exit.
    """
    yield "WARNING: no logs are available with the '{}' log driver\n".format(
        container.log_driver)


def build_log_generator(container, log_args):
    # if the container doesn't have a log_stream we need to attach to container
    # before log printer starts running
    if container.log_stream is None:
        stream = container.logs(stdout=True, stderr=True, stream=True, **log_args)
    else:
        stream = container.log_stream

    return split_buffer(stream)


def wait_on_exit(container):
    exit_code = container.wait()
    return "%s exited with code %s\n" % (container.name, exit_code)


def start_producer_thread(thread_map, event_stream, presenters, thread_args):
    queue, log_args = thread_args

    def watch_events():
        for event in event_stream:
            # TODO: handle start and stop events
            pass

    producer = Thread(target=watch_events)
    producer.daemon = True
    producer.start()


def consume_queue(queue, cascade_stop):
    """Consume the queue by reading lines off of it and yielding them."""
    while True:
        try:
            item, exception = queue.get(timeout=0.1)
        except Empty:
            pass
        # See https://github.com/docker/compose/issues/189
        except thread.error:
            raise ShutdownException()

        if exception:
            raise exception

        if item is STOP:
            if cascade_stop:
                raise StopIteration
            else:
                continue

        yield item
