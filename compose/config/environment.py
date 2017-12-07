from __future__ import absolute_import
from __future__ import unicode_literals

import codecs
import contextlib
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
    elif not os.path.isfile(filename):
        raise ConfigurationError("%s is not a file." % (filename))
    env = {}
    with contextlib.closing(codecs.open(filename, 'r', 'utf-8-sig')) as fileobj:
        for line in fileobj:
            line = line.strip()
            if line and not line.startswith('#'):
                k, v = split_env(line)
                env[k] = v
    return env


class Environment(dict):
    def __init__(self, *args, **kwargs):
        super(Environment, self).__init__(*args, **kwargs)
        self.missing_keys = []

    @classmethod
    def from_env_file(cls, base_dir):
        def _initialize():
            result = cls()
            if base_dir is None:
                return result
            env_file_path = os.path.join(base_dir, '.env')
            try:
                return cls(env_vars_from_file(env_file_path))
            except ConfigurationError:
                pass
            return result
        instance = _initialize()
        instance.update(os.environ)
        return instance

    @classmethod
    def from_command_line(cls, parsed_env_opts):
        result = cls()
        for k, v in parsed_env_opts.items():
            # Values from the command line take priority, unless they're unset
            # in which case they take the value from the system's environment
            if v is None and k in os.environ:
                result[k] = os.environ[k]
            else:
                result[k] = v
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

    def __contains__(self, key):
        result = super(Environment, self).__contains__(key)
        if IS_WINDOWS_PLATFORM:
            return (
                result or super(Environment, self).__contains__(key.upper())
            )
        return result

    def get(self, key, *args, **kwargs):
        if IS_WINDOWS_PLATFORM:
            return super(Environment, self).get(
                key,
                super(Environment, self).get(key.upper(), *args, **kwargs)
            )
        return super(Environment, self).get(key, *args, **kwargs)

    def get_boolean(self, key):
        # Convert a value to a boolean using "common sense" rules.
        # Unset, empty, "0" and "false" (i-case) yield False.
        # All other values yield True.
        value = self.get(key)
        if not value:
            return False
        if value.lower() in ['0', 'false']:
            return False
        return True
