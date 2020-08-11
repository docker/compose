import functools
import logging
import pprint
from itertools import chain


def format_call(args, kwargs):
    args = (repr(a) for a in args)
    kwargs = ("{!s}={!r}".format(*item) for item in kwargs.items())
    return "({})".format(", ".join(chain(args, kwargs)))


def format_return(result, max_lines):
    if isinstance(result, (list, tuple, set)):
        return "({} with {} items)".format(type(result).__name__, len(result))

    if result:
        lines = pprint.pformat(result).split('\n')
        extra = '\n...' if len(lines) > max_lines else ''
        return '\n'.join(lines[:max_lines]) + extra

    return result


class VerboseProxy:
    """Proxy all function calls to another class and log method name, arguments
    and return values for each call.
    """

    def __init__(self, obj_name, obj, log_name=None, max_lines=10):
        self.obj_name = obj_name
        self.obj = obj
        self.max_lines = max_lines
        self.log = logging.getLogger(log_name or __name__)

    def __getattr__(self, name):
        attr = getattr(self.obj, name)

        if not callable(attr):
            return attr

        return functools.partial(self.proxy_callable, name)

    def proxy_callable(self, call_name, *args, **kwargs):
        self.log.info("%s %s <- %s",
                      self.obj_name,
                      call_name,
                      format_call(args, kwargs))

        result = getattr(self.obj, call_name)(*args, **kwargs)
        self.log.info("%s %s -> %s",
                      self.obj_name,
                      call_name,
                      format_return(result, self.max_lines))
        return result
