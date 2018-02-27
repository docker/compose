from __future__ import absolute_import
from __future__ import unicode_literals

import codecs
import hashlib
import json
import json.decoder
import logging
import ntpath

import six
from docker.errors import DockerException
from docker.utils import parse_bytes as sdk_parse_bytes

from .errors import StreamParseError
from .timeparse import MULTIPLIERS
from .timeparse import timeparse


json_decoder = json.JSONDecoder()
log = logging.getLogger(__name__)


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
        try:
            yield decoder(buffered)
        except Exception as e:
            log.error(
                'Compose tried decoding the following data chunk, but failed:'
                '\n%s' % repr(buffered)
            )
            raise StreamParseError(e)


def json_splitter(buffer):
    """Attempt to parse a json object from a buffer. If there is at least one
    object, return it and the rest of the buffer, otherwise return None.
    """
    buffer = buffer.strip()
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
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'), default=lambda x: x.repr())
    h = hashlib.sha256()
    h.update(dump.encode('utf8'))
    return h.hexdigest()


def microseconds_from_time_nano(time_nano):
    return int(time_nano % 1000000000 / 1000)


def nanoseconds_from_time_seconds(time_seconds):
    return int(time_seconds / MULTIPLIERS['nano'])


def parse_seconds_float(value):
    return timeparse(value or '')


def parse_nanoseconds_int(value):
    parsed = timeparse(value or '')
    if parsed is None:
        return None
    return nanoseconds_from_time_seconds(parsed)


def build_string_dict(source_dict):
    return dict((k, str(v if v is not None else '')) for k, v in source_dict.items())


def splitdrive(path):
    if len(path) == 0:
        return ('', '')
    if path[0] in ['.', '\\', '/', '~']:
        return ('', path)
    return ntpath.splitdrive(path)


def parse_bytes(n):
    try:
        return sdk_parse_bytes(n)
    except DockerException:
        return None


def unquote_path(s):
    if not s:
        return s
    if s[0] == '"' and s[-1] == '"':
        return s[1:-1]
    return s
