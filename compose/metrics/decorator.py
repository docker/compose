import functools

from compose.metrics.client import MetricsCommand
from compose.metrics.client import Status


class metrics:
    def __init__(self, command_name=None):
        self.command_name = command_name

    def __call__(self, fn):
        @functools.wraps(fn,
                         assigned=functools.WRAPPER_ASSIGNMENTS,
                         updated=functools.WRAPPER_UPDATES)
        def wrapper(*args, **kwargs):
            if not self.command_name:
                self.command_name = fn.__name__
            result = fn(*args, **kwargs)
            MetricsCommand(self.command_name, status=Status.SUCCESS).send_metrics()
            return result
        return wrapper
