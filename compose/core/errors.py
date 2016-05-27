from __future__ import absolute_import
from __future__ import unicode_literals


class UpstreamError(Exception):
    pass


class NoSuchService(Exception):
    def __init__(self, name):
        self.name = name
        self.msg = "No such service: %s" % self.name

    def __str__(self):
        return self.msg


class ProjectError(Exception):
    def __init__(self, msg):
        self.msg = msg


class BuildError(Exception):
    def __init__(self, service, reason):
        self.service = service
        self.reason = reason


class NeedsBuildError(Exception):
    def __init__(self, service):
        self.service = service


class NoSuchImageError(Exception):
    pass
