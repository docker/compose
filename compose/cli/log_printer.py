import _thread as thread
import sys
from collections import namedtuple
from itertools import cycle
from operator import attrgetter
from queue import Empty
from queue import Queue
from threading import Thread

from docker.errors import APIError

from . import colors
from compose.cli.signals import ShutdownException
from compose.utils import split_buffer


class LogPresenter:

    def __init__(self, prefix_width, color_func, keep_prefix=True):
        self.prefix_width = prefix_width
        self.color_func = color_func
        self.keep_prefix = keep_prefix

    def present(self, container, line):
        to_log = '{line}'.format(line=line)

        if self.keep_prefix:
            prefix = container.name_without_project.ljust(self.prefix_width)
            to_log = '{prefix} '.format(prefix=self.color_func(prefix + ' |')) + to_log

        return to_log


def build_log_presenters(service_names, monochrome, keep_prefix=True):
    """Return an iterable of functions.

    Each function can be used to format the logs output of a container.
    """
    prefix_width = max_name_width(service_names)

    def no_color(text):
        return text

    for color_func in cycle([no_color] if monochrome else colors.rainbow()):
        yield LogPresenter(prefix_width, color_func, keep_prefix)


def max_name_width(service_names, max_index_width=3):
    """Calculate the maximum width of container names so we can make the log
    prefixes line up like so:

    db_1  | Listening
    web_1 | Listening
    """
    return max(len(name) for name in service_names) + max_index_width


class LogPrinter:
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
        self.output = output
        self.cascade_stop = cascade_stop
        self.log_args = log_args or {}

    def run(self):
        if not self.containers:
            return

        queue = Queue()
        thread_args = queue, self.log_args
        thread_map = build_thread_map(self.containers, self.presenters, thread_args)
        start_producer_thread((
            thread_map,
            self.event_stream,
            self.presenters,
            thread_args))

        for line in consume_queue(queue, self.cascade_stop):
            remove_stopped_threads(thread_map)

            if self.cascade_stop:
                matching_container = [cont.name for cont in self.containers if cont.name == line]
                if line in matching_container:
                    # Returning the name of the container that started the
                    # the cascade_stop so we can return the correct exit code
                    return line

            if not line:
                if not thread_map:
                    # There are no running containers left to tail, so exit
                    return
                # We got an empty line because of a timeout, but there are still
                # active containers to tail, so continue
                continue

            self.write(line)

    def write(self, line):
        try:
            self.output.write(line)
        except UnicodeEncodeError:
            # This may happen if the user's locale settings don't support UTF-8
            # and UTF-8 characters are present in the log line. The following
            # will output a "degraded" log with unsupported characters
            # replaced by `?`
            self.output.write(line.encode('ascii', 'replace').decode())
        self.output.flush()


def remove_stopped_threads(thread_map):
    for container_id, tailer_thread in list(thread_map.items()):
        if not tailer_thread.is_alive():
            thread_map.pop(container_id, None)


def build_thread(container, presenter, queue, log_args):
    tailer = Thread(
        target=tail_container_logs,
        args=(container, presenter, queue, log_args))
    tailer.daemon = True
    tailer.start()
    return tailer


def build_thread_map(initial_containers, presenters, thread_args):
    return {
        container.id: build_thread(container, next(presenters), *thread_args)
        # Container order is unspecified, so they are sorted by name in order to make
        # container:presenter (log color) assignment deterministic when given a list of containers
        # with the same names.
        for container in sorted(initial_containers, key=attrgetter('name'))
    }


class QueueItem(namedtuple('_QueueItem', 'item is_stop exc')):

    @classmethod
    def new(cls, item):
        return cls(item, None, None)

    @classmethod
    def exception(cls, exc):
        return cls(None, None, exc)

    @classmethod
    def stop(cls, item=None):
        return cls(item, True, None)


def tail_container_logs(container, presenter, queue, log_args):
    try:
        for item in build_log_generator(container, log_args):
            queue.put(QueueItem.new(presenter.present(container, item)))
    except Exception as e:
        queue.put(QueueItem.exception(e))
        return
    if log_args.get('follow'):
        queue.put(QueueItem.new(presenter.color_func(wait_on_exit(container))))
    queue.put(QueueItem.stop(container.name))


def build_log_generator(container, log_args):
    # if the container doesn't have a log_stream we need to attach to container
    # before log printer starts running
    if container.log_stream is None:
        stream = container.logs(stdout=True, stderr=True, stream=True, **log_args)
    else:
        stream = container.log_stream

    return split_buffer(stream)


def wait_on_exit(container):
    try:
        exit_code = container.wait()
        return "{} exited with code {}\n".format(container.name, exit_code)
    except APIError as e:
        return "Unexpected API error for {} (HTTP code {})\nResponse body:\n{}\n".format(
            container.name, e.response.status_code,
            e.response.text or '[empty]'
        )


def start_producer_thread(thread_args):
    producer = Thread(target=watch_events, args=thread_args)
    producer.daemon = True
    producer.start()


def watch_events(thread_map, event_stream, presenters, thread_args):
    crashed_containers = set()
    for event in event_stream:
        if event['action'] == 'stop':
            thread_map.pop(event['id'], None)

        if event['action'] == 'die':
            thread_map.pop(event['id'], None)
            crashed_containers.add(event['id'])

        if event['action'] != 'start':
            continue

        if event['id'] in thread_map:
            if thread_map[event['id']].is_alive():
                continue
            # Container was stopped and started, we need a new thread
            thread_map.pop(event['id'], None)

        # Container crashed so we should reattach to it
        if event['id'] in crashed_containers:
            container = event['container']
            if not container.is_restarting:
                try:
                    container.attach_log_stream()
                except APIError:
                    # Just ignore errors when reattaching to already crashed containers
                    pass
            crashed_containers.remove(event['id'])

        thread_map[event['id']] = build_thread(
            event['container'],
            next(presenters),
            *thread_args
        )


def consume_queue(queue, cascade_stop):
    """Consume the queue by reading lines off of it and yielding them."""
    while True:
        try:
            item = queue.get(timeout=0.1)
        except Empty:
            yield None
            continue
        # See https://github.com/docker/compose/issues/189
        except thread.error:
            raise ShutdownException()

        if item.exc:
            raise item.exc

        if item.is_stop and not cascade_stop:
            continue

        yield item.item
