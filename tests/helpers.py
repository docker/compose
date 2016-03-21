from __future__ import absolute_import
from __future__ import unicode_literals

from compose.config.config import ConfigDetails
from compose.config.config import ConfigFile
from compose.config.config import load


def build_config(contents, **kwargs):
    return load(build_config_details(contents, **kwargs))


def build_config_details(contents, working_dir='working_dir', filename='filename.yml'):
    return ConfigDetails(
        working_dir,
        [ConfigFile(filename, contents)],
    )
