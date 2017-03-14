from __future__ import absolute_import
from __future__ import unicode_literals

import signal

from ..const import IS_WINDOWS_PLATFORM


class ShutdownException(Exception):
    pass


def shutdown(signal, frame):
    raise ShutdownException()


def set_signal_handler(handler):
    signal.signal(signal.SIGINT, handler)
    signal.signal(signal.SIGTERM, handler)


def set_signal_handler_to_shutdown():
    set_signal_handler(shutdown)


def ignore_sigpipe():
    # Restore default behavior for SIGPIPE instead of raising
    # an exception when encountered.
    if not IS_WINDOWS_PLATFORM:
        signal.signal(signal.SIGPIPE, signal.SIG_DFL)
