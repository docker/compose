from __future__ import absolute_import
from __future__ import unicode_literals


VERSION_EXPLANATION = (
    'You might be seeing this error because you\'re using the wrong Compose file version. '
    'Either specify a supported version ("2.0", "2.1", "3.0", "3.1", "3.2") and place '
    'your service definitions under the `services` key, or omit the `version` key '
    'and place your service definitions at the root of the file to use '
    'version 1.\nFor more on the Compose file format versions, see '
    'https://docs.docker.com/compose/compose-file/')


class ConfigurationError(Exception):
    def __init__(self, msg):
        self.msg = msg

    def __str__(self):
        return self.msg


class DependencyError(ConfigurationError):
    pass


class CircularReference(ConfigurationError):
    def __init__(self, trail):
        self.trail = trail

    @property
    def msg(self):
        lines = [
            "{} in {}".format(service_name, filename)
            for (filename, service_name) in self.trail
        ]
        return "Circular reference:\n  {}".format("\n  extends ".join(lines))


class ComposeFileNotFound(ConfigurationError):
    def __init__(self, supported_filenames):
        super(ComposeFileNotFound, self).__init__("""
        Can't find a suitable configuration file in this directory or any
        parent. Are you in the right directory?

        Supported filenames: %s
        """ % ", ".join(supported_filenames))


class DuplicateOverrideFileFound(ConfigurationError):
    def __init__(self, override_filenames):
        self.override_filenames = override_filenames
        super(DuplicateOverrideFileFound, self).__init__(
            "Multiple override files found: {}. You may only use a single "
            "override file.".format(", ".join(override_filenames))
        )
