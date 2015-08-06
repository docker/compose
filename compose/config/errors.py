class ConfigurationError(Exception):
    def __init__(self, msg):
        self.msg = msg

    def __str__(self):
        return self.msg


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
        Can't find a suitable configuration file in this directory or any parent. Are you in the right directory?

        Supported filenames: %s
        """ % ", ".join(supported_filenames))
