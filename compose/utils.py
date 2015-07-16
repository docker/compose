import codecs
import hashlib
import json
import logging
import os
import sys

from docker.errors import APIError
import concurrent.futures

from .const import DEFAULT_MAX_WORKERS


log = logging.getLogger(__name__)


def parallel_execute(command, containers, doing_msg, done_msg, **options):
    """
    Execute a given command upon a list of containers in parallel.
    """
    max_workers = os.environ.get('COMPOSE_MAX_WORKERS', DEFAULT_MAX_WORKERS)
    stream = codecs.getwriter('utf-8')(sys.stdout)
    lines = []
    errors = {}

    for container in containers:
        write_out_msg(stream, lines, container.name, doing_msg)

    def container_command_execute(container, command, **options):
        try:
            getattr(container, command)(**options)
        except APIError as e:
            errors[container.name] = e.explanation

    with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
        future_container = {
            executor.submit(
                container_command_execute,
                container,
                command,
                **options
            ): container for container in containers
        }

        for future in concurrent.futures.as_completed(future_container):
            container = future_container[future]
            write_out_msg(stream, lines, container.name, done_msg)

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
