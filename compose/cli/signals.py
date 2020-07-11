import signal

from ..const import IS_WINDOWS_PLATFORM


class ShutdownException(Exception):
    pass


class HangUpException(Exception):
    pass


def shutdown(signal, frame):
    raise ShutdownException()


def set_signal_handler(handler):
    signal.signal(signal.SIGINT, handler)
    signal.signal(signal.SIGTERM, handler)


def set_signal_handler_to_shutdown():
    set_signal_handler(shutdown)


def hang_up(signal, frame):
    raise HangUpException()


def set_signal_handler_to_hang_up():
    # on Windows a ValueError will be raised if trying to set signal handler for SIGHUP
    if not IS_WINDOWS_PLATFORM:
        signal.signal(signal.SIGHUP, hang_up)


def ignore_sigpipe():
    # Restore default behavior for SIGPIPE instead of raising
    # an exception when encountered.
    if not IS_WINDOWS_PLATFORM:
        signal.signal(signal.SIGPIPE, signal.SIG_DFL)
