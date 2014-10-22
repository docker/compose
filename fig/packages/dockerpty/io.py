# dockerpty: io.py
#
# Copyright 2014 Chris Corbyn <chris@w3style.co.uk>
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import fcntl
import errno
import struct
import select as builtin_select


def set_blocking(fd, blocking=True):
    """
    Set the given file-descriptor blocking or non-blocking.

    Returns the original blocking status.
    """

    old_flag = fcntl.fcntl(fd, fcntl.F_GETFL)

    if blocking:
        new_flag = old_flag &~ os.O_NONBLOCK
    else:
        new_flag = old_flag | os.O_NONBLOCK

    fcntl.fcntl(fd, fcntl.F_SETFL, new_flag)

    return not bool(old_flag & os.O_NONBLOCK)


def select(read_streams, timeout=0):
    """
    Select the streams from `read_streams` that are ready for reading.

    Uses `select.select()` internally but returns a flat list of streams.
    """

    write_streams = []
    exception_streams = []

    try:
        return builtin_select.select(
            read_streams,
            write_streams,
            exception_streams,
            timeout,
        )[0]
    except builtin_select.error as e:
        # POSIX signals interrupt select()
        if e[0] == errno.EINTR:
            return []
        else:
            raise e


class Stream(object):
    """
    Generic Stream class.

    This is a file-like abstraction on top of os.read() and os.write(), which
    add consistency to the reading of sockets and files alike.
    """


    """
    Recoverable IO/OS Errors.
    """
    ERRNO_RECOVERABLE = [
        errno.EINTR,
        errno.EDEADLK,
        errno.EWOULDBLOCK,
    ]


    def __init__(self, fd):
        """
        Initialize the Stream for the file descriptor `fd`.

        The `fd` object must have a `fileno()` method.
        """
        self.fd = fd


    def fileno(self):
        """
        Return the fileno() of the file descriptor.
        """

        return self.fd.fileno()


    def set_blocking(self, value):
        if hasattr(self.fd, 'setblocking'):
            self.fd.setblocking(value)
            return True
        else:
            return set_blocking(self.fd, value)


    def read(self, n=4096):
        """
        Return `n` bytes of data from the Stream, or None at end of stream.
        """

        try:
            if hasattr(self.fd, 'recv'):
                return self.fd.recv(n)
            return os.read(self.fd.fileno(), n)
        except EnvironmentError as e:
            if e.errno not in Stream.ERRNO_RECOVERABLE:
                raise e


    def write(self, data):
        """
        Write `data` to the Stream.
        """

        if not data:
            return None

        while True:
            try:
                if hasattr(self.fd, 'send'):
                    self.fd.send(data)
                    return len(data)
                os.write(self.fd.fileno(), data)
                return len(data)
            except EnvironmentError as e:
                if e.errno not in Stream.ERRNO_RECOVERABLE:
                    raise e

    def __repr__(self):
        return "{cls}({fd})".format(cls=type(self).__name__, fd=self.fd)


class Demuxer(object):
    """
    Wraps a multiplexed Stream to read in data demultiplexed.

    Docker multiplexes streams together when there is no PTY attached, by
    sending an 8-byte header, followed by a chunk of data.

    The first 4 bytes of the header denote the stream from which the data came
    (i.e. 0x01 = stdout, 0x02 = stderr). Only the first byte of these initial 4
    bytes is used.

    The next 4 bytes indicate the length of the following chunk of data as an
    integer in big endian format. This much data must be consumed before the
    next 8-byte header is read.
    """

    def __init__(self, stream):
        """
        Initialize a new Demuxer reading from `stream`.
        """

        self.stream = stream
        self.remain = 0


    def fileno(self):
        """
        Returns the fileno() of the underlying Stream.

        This is useful for select() to work.
        """

        return self.stream.fileno()


    def set_blocking(self, value):
        return self.stream.set_blocking(value)


    def read(self, n=4096):
        """
        Read up to `n` bytes of data from the Stream, after demuxing.

        Less than `n` bytes of data may be returned depending on the available
        payload, but the number of bytes returned will never exceed `n`.

        Because demuxing involves scanning 8-byte headers, the actual amount of
        data read from the underlying stream may be greater than `n`.
        """

        size = self._next_packet_size(n)

        if size <= 0:
            return
        else:
            return self.stream.read(size)


    def write(self, data):
        """
        Delegates the the underlying Stream.
        """

        return self.stream.write(data)


    def _next_packet_size(self, n=0):
        size = 0

        if self.remain > 0:
            size = min(n, self.remain)
            self.remain -= size
        else:
            data = self.stream.read(8)
            if data is None:
                return 0
            if len(data) == 8:
                __, actual = struct.unpack('>BxxxL', data)
                size = min(n, actual)
                self.remain = actual - size

        return size

    def __repr__(self):
        return "{cls}({stream})".format(cls=type(self).__name__,
                                        stream=self.stream)


class Pump(object):
    """
    Stream pump class.

    A Pump wraps two Streams, reading from one and and writing its data into
    the other, much like a pipe but manually managed.

    This abstraction is used to facilitate piping data between the file
    descriptors associated with the tty and those associated with a container's
    allocated pty.

    Pumps are selectable based on the 'read' end of the pipe.
    """

    def __init__(self, from_stream, to_stream):
        """
        Initialize a Pump with a Stream to read from and another to write to.
        """

        self.from_stream = from_stream
        self.to_stream = to_stream


    def fileno(self):
        """
        Returns the `fileno()` of the reader end of the Pump.

        This is useful to allow Pumps to function with `select()`.
        """

        return self.from_stream.fileno()


    def set_blocking(self, value):
        return self.from_stream.set_blocking(value)


    def flush(self, n=4096):
        """
        Flush `n` bytes of data from the reader Stream to the writer Stream.

        Returns the number of bytes that were actually flushed. A return value
        of zero is not an error.

        If EOF has been reached, `None` is returned.
        """

        try:
            return self.to_stream.write(self.from_stream.read(n))
        except OSError as e:
            if e.errno != errno.EPIPE:
                raise e

    def __repr__(self):
        return "{cls}(from={from_stream}, to={to_stream})".format(
            cls=type(self).__name__,
            from_stream=self.from_stream,
            to_stream=self.to_stream)
