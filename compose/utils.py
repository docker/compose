import codecs
import hashlib
import json
import logging
import sys

from docker.errors import APIError
from Queue import Queue, Empty
from threading import Thread


log = logging.getLogger(__name__)


def parallel_execute(objects, obj_callable, msg_index, msg):
    """
    For a given list of objects, call the callable passing in the first
    object we give it.
    """
    stream = codecs.getwriter('utf-8')(sys.stdout)
    lines = []
    errors = {}

    for obj in objects:
        write_out_msg(stream, lines, msg_index(obj), msg)

    q = Queue()

    def inner_execute_function(an_callable, parameter, msg_index):
        try:
            result = an_callable(parameter)
        except APIError as e:
            errors[msg_index] = e.explanation
            result = "error"
        except Exception as e:
            errors[msg_index] = e
            result = 'unexpected_exception'

        q.put((msg_index, result))

    for an_object in objects:
        t = Thread(
            target=inner_execute_function,
            args=(obj_callable, an_object, msg_index(an_object)),
        )
        t.daemon = True
        t.start()

    done = 0
    total_to_execute = len(objects)

    while done < total_to_execute:
        try:
            msg_index, result = q.get(timeout=1)

            if result == 'unexpected_exception':
                raise errors[msg_index]
            if result == 'error':
                write_out_msg(stream, lines, msg_index, msg, status='error')
            else:
                write_out_msg(stream, lines, msg_index, msg)
            done += 1
        except Empty:
            pass

    if errors:
        stream.write("\n")
        for error in errors:
            stream.write("ERROR: for {}  {} \n".format(error, errors[error]))


def write_out_msg(stream, lines, msg_index, msg, status="done"):
    """
    Using special ANSI code characters we can write out the msg over the top of
    a previous status message, if it exists.
    """
    obj_index = msg_index
    if msg_index in lines:
        position = lines.index(obj_index)
        diff = len(lines) - position
        # move up
        stream.write("%c[%dA" % (27, diff))
        # erase
        stream.write("%c[2K\r" % 27)
        stream.write("{} {}... {}\n".format(msg, obj_index, status))
        # move back down
        stream.write("%c[%dB" % (27, diff))
    else:
        diff = 0
        lines.append(obj_index)
        stream.write("{} {}... \r\n".format(msg, obj_index))

    stream.flush()


def json_hash(obj):
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'))
    h = hashlib.sha256()
    h.update(dump)
    return h.hexdigest()
