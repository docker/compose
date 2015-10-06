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


def parallel_execute(objects, obj_callable, msg_index, msg):
    """
    For a given list of objects, call the callable passing in the first
    object we give it.
    """
    stream = get_output_stream(sys.stdout)
    lines = []

    for obj in objects:
        write_out_msg(stream, lines, msg_index(obj), msg)

    q = Queue()

    def inner_execute_function(an_callable, parameter, msg_index):
        error = None
        try:
            result = an_callable(parameter)
        except APIError as e:
            error = e.explanation
            result = "error"
        except Exception as e:
            error = e
            result = 'unexpected_exception'

        q.put((msg_index, result, error))

    for an_object in objects:
        t = Thread(
            target=inner_execute_function,
            args=(obj_callable, an_object, msg_index(an_object)),
        )
        t.daemon = True
        t.start()

    done = 0
    errors = {}
    total_to_execute = len(objects)

    while done < total_to_execute:
        try:
            msg_index, result, error = q.get(timeout=1)

            if result == 'unexpected_exception':
                errors[msg_index] = result, error
            if result == 'error':
                errors[msg_index] = result, error
                write_out_msg(stream, lines, msg_index, msg, status='error')
            else:
                write_out_msg(stream, lines, msg_index, msg)
            done += 1
        except Empty:
            pass

    if not errors:
        return

    stream.write("\n")
    for msg_index, (result, error) in errors.items():
        stream.write("ERROR: for {}  {} \n".format(msg_index, error))
        if result == 'unexpected_exception':
            raise error


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
            data = data.decode('utf-8')
        yield data


def line_splitter(buffer, separator=u'\n'):
    index = buffer.find(six.text_type(separator))
    if index == -1:
        return None, None
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
            item, rest = splitter(buffered)
            if not item:
                break

            buffered = rest
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
        return None, None


def json_stream(stream):
    """Given a stream of text, return a stream of json objects.
    This handles streams which are inconsistently buffered (some entries may
    be newline delimited, and others are not).
    """
    return split_buffer(stream_as_text(stream), json_splitter, json_decoder.decode)


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
        stream.write("{} {} ... {}\n".format(msg, obj_index, status))
        # move back down
        stream.write("%c[%dB" % (27, diff))
    else:
        diff = 0
        lines.append(obj_index)
        stream.write("{} {} ... \r\n".format(msg, obj_index))

    stream.flush()


def json_hash(obj):
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'))
    h = hashlib.sha256()
    h.update(dump.encode('utf8'))
    return h.hexdigest()
