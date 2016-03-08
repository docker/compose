from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os

from .errors import ConfigurationError

log = logging.getLogger(__name__)


class BlankDefaultDict(dict):
    def __init__(self, *args, **kwargs):
        super(BlankDefaultDict, self).__init__(*args, **kwargs)
        self.missing_keys = []

    def __getitem__(self, key):
        try:
            return super(BlankDefaultDict, self).__getitem__(key)
        except KeyError:
            if key not in self.missing_keys:
                log.warn(
                    "The {} variable is not set. Defaulting to a blank string."
                    .format(key)
                )
                self.missing_keys.append(key)

            return ""


class Environment(BlankDefaultDict):
    def __init__(self, base_dir):
        super(Environment, self).__init__()
        if base_dir:
            self.load_environment_file(os.path.join(base_dir, '.env'))
        self.update(os.environ)

    def load_environment_file(self, path):
        if not os.path.exists(path):
            return
        mapping = {}
        with open(path, 'r') as f:
            for line in f.readlines():
                line = line.strip()
                if '=' not in line:
                    raise ConfigurationError(
                        'Invalid environment variable mapping in env file. '
                        'Missing "=" in "{0}"'.format(line)
                    )
                mapping.__setitem__(*line.split('=', 1))
        self.update(mapping)
