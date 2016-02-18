from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os
from string import Template

import six

from .errors import ConfigurationError
log = logging.getLogger(__name__)


def interpolate_environment_variables(config, section):
    mapping = BlankDefaultDict(os.environ)

    try:
        return recursive_interpolate(config, mapping)
    except InvalidInterpolation as e:
        raise ConfigurationError(
            'Invalid interpolation format ("{string}") in "{section}"'.format(
                section=section,
                string=e.string))


def recursive_interpolate(obj, mapping):
    if isinstance(obj, six.string_types):
        return interpolate(obj, mapping)
    elif isinstance(obj, dict):
        return dict(
            (interpolate(key, mapping), recursive_interpolate(val, mapping))
            for (key, val) in obj.items()
        )
    elif isinstance(obj, list):
        return [recursive_interpolate(val, mapping) for val in obj]
    else:
        return obj


def interpolate(string, mapping):
    try:
        return Template(string).substitute(mapping)
    except ValueError:
        raise InvalidInterpolation(string)


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


class InvalidInterpolation(Exception):
    def __init__(self, string):
        self.string = string
