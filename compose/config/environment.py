from __future__ import absolute_import
from __future__ import unicode_literals

import codecs
import logging
import os

import six

from ..const import IS_WINDOWS_PLATFORM
from .errors import ConfigurationError

log = logging.getLogger(__name__)


def split_env(env):
    if isinstance(env, six.binary_type):
        env = env.decode('utf-8', 'replace')
    if '=' in env:
        return env.split('=', 1)
    else:
        return env, None


def env_vars_from_file(filename):
    """
    Read in a line delimited file of environment variables.
    """
    if not os.path.exists(filename):
        raise ConfigurationError("Couldn't find env file: %s" % filename)
    env = {}
    for line in codecs.open(filename, 'r', 'utf-8'):
        line = line.strip()
        if line and not line.startswith('#'):
            k, v = split_env(line)
            env[k] = v
    return env


class Environment(dict):
    def __init__(self, *args, **kwargs):
        super(Environment, self).__init__(*args, **kwargs)
        self.missing_keys = []
        self.update(os.environ)

    @classmethod
    def from_env_file(cls, base_dir):
        result = cls()
        if base_dir is None:
            return result
        env_file_path = os.path.join(base_dir, '.env')
        try:
            return cls(env_vars_from_file(env_file_path))
        except ConfigurationError:
            pass
        return result

    def __getitem__(self, key):
        try:
            return super(Environment, self).__getitem__(key)
        except KeyError:
            if IS_WINDOWS_PLATFORM:
                try:
                    return super(Environment, self).__getitem__(key.upper())
                except KeyError:
                    pass
            if key not in self.missing_keys:
                log.warn(
                    "The {} variable is not set. Defaulting to a blank string."
                    .format(key)
                )
                self.missing_keys.append(key)

            return ""
