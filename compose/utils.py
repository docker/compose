import codecs
import hashlib
import json
import json.decoder
import logging
import sys
from threading import Thread

import six
from docker.errors import APIError
from six.moves.queue import Empty
from six.moves.queue import Queue


log = logging.getLogger(__name__)

json_decoder = json.JSONDecoder()


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


def get_output_stream(stream):
    if six.PY3:
        return stream
    return codecs.getwriter('utf-8')(stream)


def stream_as_text(stream):
    """Given a stream of bytes or text, if any of the items in the stream
    are bytes convert them to text.

    This function can be removed once docker-py returns text streams instead
    of byte streams.
    """
    for data in stream:
        if not isinstance(data, six.text_type):
            data = data.decode('utf-8', 'replace')
        yield data


def line_splitter(buffer, separator=u'\n'):
    index = buffer.find(six.text_type(separator))
    if index == -1:
        return None
    return buffer[:index + 1], buffer[index + 1:]


def split_buffer(stream, splitter=None, decoder=lambda a: a):
    """Given a generator which yields strings and a splitter function,
    joins all input, splits on the separator and yields each chunk.

    Unlike string.split(), each chunk includes the trailing
    separator, except for the last one if none was found on the end
    of the input.
    """
    splitter = splitter or line_splitter
    buffered = six.text_type('')

    for data in stream_as_text(stream):
        buffered += data
        while True:
            buffer_split = splitter(buffered)
            if buffer_split is None:
                break

            item, buffered = buffer_split
            yield item

    if buffered:
        yield decoder(buffered)


def json_splitter(buffer):
    """Attempt to parse a json object from a buffer. If there is at least one
    object, return it and the rest of the buffer, otherwise return None.
    """
    try:
        obj, index = json_decoder.raw_decode(buffer)
        rest = buffer[json.decoder.WHITESPACE.match(buffer, index).end():]
        return obj, rest
    except ValueError:
        return None


def json_stream(stream):
    """Given a stream of text, return a stream of json objects.
    This handles streams which are inconsistently buffered (some entries may
    be newline delimited, and others are not).
    """
    return split_buffer(stream, json_splitter, json_decoder.decode)


def json_hash(obj):
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'))
    h = hashlib.sha256()
    h.update(dump.encode('utf8'))
    return h.hexdigest()
