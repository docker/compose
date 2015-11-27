from __future__ import absolute_import
from __future__ import unicode_literals

import operator
import sys
from threading import Thread

from docker.errors import APIError
from six.moves.queue import Empty
from six.moves.queue import Queue

from compose.utils import get_output_stream


def perform_operation(func, arg, callback, index):
    try:
        callback((index, func(arg)))
    except Exception as e:
        callback((index, e))


def parallel_execute(objects, func, index_func, msg):
    """For a given list of objects, call the callable passing in the first
    object we give it.
    """
    objects = list(objects)
    stream = get_output_stream(sys.stdout)
    writer = ParallelStreamWriter(stream, msg)

    for obj in objects:
        writer.initialize(index_func(obj))

    q = Queue()

    # TODO: limit the number of threads #1828
    for obj in objects:
        t = Thread(
            target=perform_operation,
            args=(func, obj, q.put, index_func(obj)))
        t.daemon = True
        t.start()

    done = 0
    errors = {}

    while done < len(objects):
        try:
            msg_index, result = q.get(timeout=1)
        except Empty:
            continue

        if isinstance(result, APIError):
            errors[msg_index] = "error", result.explanation
            writer.write(msg_index, 'error')
        elif isinstance(result, Exception):
            errors[msg_index] = "unexpected_exception", result
        else:
            writer.write(msg_index, 'done')
        done += 1

    if not errors:
        return

    stream.write("\n")
    for msg_index, (result, error) in errors.items():
        stream.write("ERROR: for {}  {} \n".format(msg_index, error))
        if result == 'unexpected_exception':
            raise error


class ParallelStreamWriter(object):
    """Write out messages for operations happening in parallel.

    Each operation has it's own line, and ANSI code characters are used
    to jump to the correct line, and write over the line.
    """

    def __init__(self, stream, msg):
        self.stream = stream
        self.msg = msg
        self.lines = []

    def initialize(self, obj_index):
        self.lines.append(obj_index)
        self.stream.write("{} {} ... \r\n".format(self.msg, obj_index))
        self.stream.flush()

    def write(self, obj_index, status):
        position = self.lines.index(obj_index)
        diff = len(self.lines) - position
        # move up
        self.stream.write("%c[%dA" % (27, diff))
        # erase
        self.stream.write("%c[2K\r" % 27)
        self.stream.write("{} {} ... {}\r".format(self.msg, obj_index, status))
        # move back down
        self.stream.write("%c[%dB" % (27, diff))
        self.stream.flush()


def parallel_operation(containers, operation, options, message):
    parallel_execute(
        containers,
        operator.methodcaller(operation, **options),
        operator.attrgetter('name'),
        message)


def parallel_remove(containers, options):
    stopped_containers = [c for c in containers if not c.is_running]
    parallel_operation(stopped_containers, 'remove', options, 'Removing')


def parallel_stop(containers, options):
    parallel_operation(containers, 'stop', options, 'Stopping')


def parallel_start(containers, options):
    parallel_operation(containers, 'start', options, 'Starting')


def parallel_pause(containers, options):
    parallel_operation(containers, 'pause', options, 'Pausing')


def parallel_unpause(containers, options):
    parallel_operation(containers, 'unpause', options, 'Unpausing')


def parallel_kill(containers, options):
    parallel_operation(containers, 'kill', options, 'Killing')


def parallel_restart(containers, options):
    parallel_operation(containers, 'restart', options, 'Restarting')
