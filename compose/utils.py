import codecs
import hashlib
import json
import logging
import sys

from docker.errors import APIError
from Queue import Queue, Empty
from threading import Thread


log = logging.getLogger(__name__)


def parallel_create_execute(create_function, container_numbers, msgs={}, **options):
    """
    Parallel container creation by calling the create_function for each new container
    number passed in.
    """
    stream = codecs.getwriter('utf-8')(sys.stdout)
    lines = []
    errors = {}

    for number in container_numbers:
        write_out_msg(stream, lines, number, msgs['doing'])

    q = Queue()

    def inner_call_function(create_function, number):
        try:
            container = create_function(number)
        except APIError as e:
            errors[number] = e.explanation
        q.put(container)

    for number in container_numbers:
        t = Thread(
            target=inner_call_function,
            args=(create_function, number),
            kwargs=options,
        )
        t.daemon = True
        t.start()

    done = 0
    total_to_create = len(container_numbers)
    while done < total_to_create:
        try:
            container = q.get(timeout=1)
            write_out_msg(stream, lines, container.name, msgs['done'])
            done += 1
        except Empty:
            pass

    if errors:
        for number in errors:
            stream.write("ERROR: for {}  {} \n".format(number, errors[number]))


def parallel_execute(command, containers, doing_msg, done_msg, **options):
    """
    Execute a given command upon a list of containers in parallel.
    """
    stream = codecs.getwriter('utf-8')(sys.stdout)
    lines = []
    errors = {}

    for container in containers:
        write_out_msg(stream, lines, container.name, doing_msg)

    q = Queue()

    def container_command_execute(container, command, **options):
        try:
            getattr(container, command)(**options)
        except APIError as e:
            errors[container.name] = e.explanation
        q.put(container)

    for container in containers:
        t = Thread(
            target=container_command_execute,
            args=(container, command),
            kwargs=options,
        )
        t.daemon = True
        t.start()

    done = 0

    while done < len(containers):
        try:
            container = q.get(timeout=1)
            write_out_msg(stream, lines, container.name, done_msg)
            done += 1
        except Empty:
            pass

    if errors:
        for container in errors:
            stream.write("ERROR: for {}  {} \n".format(container, errors[container]))


def write_out_msg(stream, lines, container_name, msg):
    """
    Using special ANSI code characters we can write out the msg over the top of
    a previous status message, if it exists.
    """
    if container_name in lines:
        position = lines.index(container_name)
        diff = len(lines) - position
        # move up
        stream.write("%c[%dA" % (27, diff))
        # erase
        stream.write("%c[2K\r" % 27)
        stream.write("{}: {} \n".format(container_name, msg))
        # move back down
        stream.write("%c[%dB" % (27, diff))
    else:
        diff = 0
        lines.append(container_name)
        stream.write("{}: {}... \r\n".format(container_name, msg))

    stream.flush()


def json_hash(obj):
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'))
    h = hashlib.sha256()
    h.update(dump)
    return h.hexdigest()
