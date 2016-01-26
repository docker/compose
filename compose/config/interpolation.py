from __future__ import absolute_import
from __future__ import unicode_literals

import json
import logging
import os
from string import Template

import six

from .errors import ConfigurationError


log = logging.getLogger(__name__)


def interpolate_environment_variables(config, section, schema):
    mapping = BlankDefaultDict(os.environ)

    def process_item(name, config_dict):
        return dict(
            (key, interpolate_value(name, key, val, section, mapping, schema))
            for key, val in (config_dict or {}).items()
        )

    return dict(
        (name, process_item(name, config_dict or {}))
        for name, config_dict in config.items()
    )


def _cast_interpolated(interpolated_value, expected_type):

    if expected_type == "string":
        return None
    elif expected_type == "number":
        try:
            return int(interpolated_value)
        except ValueError:
            return None
    elif expected_type == "boolean":
        if "true" == interpolated_value.lower():
            return True
        if "false" == interpolated_value.lower():
            return False
        return None
    elif expected_type == "array":
        if isinstance(interpolated_value, list):
            return interpolated_value
        return json.loads("{{\"data\":{0}}}".format(interpolated_value.replace("'", "\"")))["data"]


def interpolate_value(name, config_key, value, section, mapping, schema):
    try:
        interpolated = recursive_interpolate(value, mapping)
        if (interpolated != value):
            # cast as needed
            if config_key in schema["definitions"]["service"]["properties"]:
                field_def = schema["definitions"]["service"]["properties"][config_key]
                if "type" in field_def:
                    allowed_types = field_def["type"]
                    if isinstance(allowed_types, list):
                        for allowed_type in allowed_types:
                            converted = _cast_interpolated(interpolated, allowed_type)
                            if converted is not None:
                                return converted
                    else:
                        converted = _cast_interpolated(interpolated, allowed_types)
                        if converted is not None:
                            return converted
        return interpolated
    except InvalidInterpolation as e:
        raise ConfigurationError(
            'Invalid interpolation format for "{config_key}" option '
            'in {section} "{name}": "{string}"'.format(
                config_key=config_key,
                name=name,
                section=section,
                string=e.string))


def recursive_interpolate(obj, mapping):
    if isinstance(obj, six.string_types):
        if '$' in obj:
            return interpolate(obj, mapping)
        return obj
    elif isinstance(obj, dict):
        return dict(
            (key, recursive_interpolate(val, mapping))
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
