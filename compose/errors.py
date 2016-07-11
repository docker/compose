from __future__ import absolute_import
from __future__ import unicode_literals


class OperationFailedError(Exception):
    def __init__(self, reason):
        self.msg = reason
