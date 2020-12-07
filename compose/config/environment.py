import logging
import os
import re

import dotenv

from ..const import IS_WINDOWS_PLATFORM
from .errors import ConfigurationError
from .errors import EnvFileNotFound

log = logging.getLogger(__name__)


def split_env(env):
    if isinstance(env, bytes):
        env = env.decode('utf-8', 'replace')
    key = value = None
    if '=' in env:
        key, value = env.split('=', 1)
    else:
        key = env
    if re.search(r'\s', key):
        raise ConfigurationError(
            "environment variable name '{}' may not contain whitespace.".format(key)
        )
    return key, value


def env_vars_from_file(filename, interpolate=True):
    """
    Read in a line delimited file of environment variables.
    """
    if not os.path.exists(filename):
        raise EnvFileNotFound("Couldn't find env file: {}".format(filename))
    elif not os.path.isfile(filename):
        raise EnvFileNotFound("{} is not a file.".format(filename))

    env = dotenv.dotenv_values(dotenv_path=filename, encoding='utf-8-sig', interpolate=interpolate)
    for k, v in env.items():
        env[k] = v if interpolate else v.replace('$', '$$')
    return env


class Environment(dict):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.missing_keys = []
        self.silent = False

    @classmethod
    def from_env_file(cls, base_dir, env_file=None):
        def _initialize():
            result = cls()
            if base_dir is None:
                return result
            if env_file:
                env_file_path = os.path.join(base_dir, env_file)
            else:
                env_file_path = os.path.join(base_dir, '.env')
            try:
                return cls(env_vars_from_file(env_file_path))
            except EnvFileNotFound:
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
            return super().__getitem__(key)
        except KeyError:
            if IS_WINDOWS_PLATFORM:
                try:
                    return super().__getitem__(key.upper())
                except KeyError:
                    pass
            if not self.silent and key not in self.missing_keys:
                log.warning(
                    "The {} variable is not set. Defaulting to a blank string."
                    .format(key)
                )
                self.missing_keys.append(key)

            return ""

    def __contains__(self, key):
        result = super().__contains__(key)
        if IS_WINDOWS_PLATFORM:
            return (
                result or super().__contains__(key.upper())
            )
        return result

    def get(self, key, *args, **kwargs):
        if IS_WINDOWS_PLATFORM:
            return super().get(
                key,
                super().get(key.upper(), *args, **kwargs)
            )
        return super().get(key, *args, **kwargs)

    def get_boolean(self, key, default=False):
        # Convert a value to a boolean using "common sense" rules.
        # Unset, empty, "0" and "false" (i-case) yield False.
        # All other values yield True.
        value = self.get(key)
        if not value:
            return default
        if value.lower() in ['0', 'false']:
            return False
        return True
