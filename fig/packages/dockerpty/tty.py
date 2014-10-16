# dockerpty: tty.py
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

from __future__ import absolute_import

import os
import termios
import tty
import fcntl
import struct


def size(fd):
    """
    Return a tuple (rows,cols) representing the size of the TTY `fd`.

    The provided file descriptor should be the stdout stream of the TTY.

    If the TTY size cannot be determined, returns None.
    """

    if not os.isatty(fd.fileno()):
        return None

    try:
        dims = struct.unpack('hh', fcntl.ioctl(fd, termios.TIOCGWINSZ, 'hhhh'))
    except:
        try:
            dims = (os.environ['LINES'], os.environ['COLUMNS'])
        except:
            return None

    return dims


class Terminal(object):
    """
    Terminal provides wrapper functionality to temporarily make the tty raw.

    This is useful when streaming data from a pseudo-terminal into the tty.

    Example:

        with Terminal(sys.stdin, raw=True):
            do_things_in_raw_mode()
    """

    def __init__(self, fd, raw=True):
        """
        Initialize a terminal for the tty with stdin attached to `fd`.

        Initializing the Terminal has no immediate side effects. The `start()`
        method must be invoked, or `with raw_terminal:` used before the
        terminal is affected.
        """

        self.fd = fd
        self.raw = raw
        self.original_attributes = None


    def __enter__(self):
        """
        Invoked when a `with` block is first entered.
        """

        self.start()
        return self


    def __exit__(self, *_):
        """
        Invoked when a `with` block is finished.
        """

        self.stop()


    def israw(self):
        """
        Returns True if the TTY should operate in raw mode.
        """

        return self.raw


    def start(self):
        """
        Saves the current terminal attributes and makes the tty raw.

        This method returns None immediately.
        """

        if os.isatty(self.fd.fileno()) and self.israw():
            self.original_attributes = termios.tcgetattr(self.fd)
            tty.setraw(self.fd)


    def stop(self):
        """
        Restores the terminal attributes back to before setting raw mode.

        If the raw terminal was not started, does nothing.
        """

        if self.original_attributes is not None:
            termios.tcsetattr(
                self.fd,
                termios.TCSADRAIN,
                self.original_attributes,
            )

    def __repr__(self):
        return "{cls}({fd}, raw={raw})".format(
            cls=type(self).__name__,
            fd=self.fd,
            raw=self.raw)
