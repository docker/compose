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


class ValidationError(ConfigurationError):
    def __init__(self, source, errors):
        error_msg = ''
        for k, v in errors.items():
            try:
                error_msg += '  {k}: {v}\n'.format(k=k, v=v)
            except UnicodeEncodeError, e:  # noqa
                try:
                    error_msg += '  {k}: value contains non-ascii character\n'.format(k=k)
                except UnicodeEncodeError, e:  # noqa
                    error_msg += '  some key contains non-ascii character\n'

        self.msg = 'Configuration errors in `{source}`:\n{errors}'.format(source=source, errors=error_msg)
