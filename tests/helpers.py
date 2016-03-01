from __future__ import absolute_import
from __future__ import unicode_literals

import functools
import os

from . import mock
from compose.config.config import ConfigDetails
from compose.config.config import ConfigFile
from compose.config.config import load
from compose.config.environment import Environment


def build_config(contents, **kwargs):
    return load(build_config_details(contents, **kwargs))


def build_config_details(contents, working_dir='working_dir', filename='filename.yml'):
    return ConfigDetails(
        working_dir,
        [ConfigFile(filename, contents)])


def clear_environment(f):
    @functools.wraps(f)
    def wrapper(self, *args, **kwargs):
        Environment.reset()
        with mock.patch.dict(os.environ):
            f(self, *args, **kwargs)
