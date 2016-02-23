from __future__ import absolute_import
from __future__ import unicode_literals

import signal


class ShutdownException(Exception):
    pass


def shutdown(signal, frame):
    raise ShutdownException()


def set_signal_handler(handler):
    signal.signal(signal.SIGINT, handler)
    signal.signal(signal.SIGTERM, handler)


def set_signal_handler_to_shutdown():
    set_signal_handler(shutdown)
