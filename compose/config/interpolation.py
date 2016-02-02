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


def _cast_interpolated_inside_list(interpolated_value, possible_types):
    if "string" in possible_types:
        possible_types.remove("string")
    for possible_type in possible_types:
        v = _cast_interpolated(interpolated_value, possible_type)
        if v is not None:
            return v
    return interpolated_value


def _cast_interpolated(interpolated_value, field_def):
    field_type = field_def
    if "type" in field_def:
        field_type = field_def["type"]
    if isinstance(field_type, list):
        return _cast_interpolated_inside_list(interpolated_value, field_type)
    if field_type == "array" and isinstance(interpolated_value, list):
        return [_cast_interpolated(subfield, field_def["items"]["type"]) for subfield in interpolated_value]
    try:
        return json.loads(interpolated_value.replace("'", "\""))
    except ValueError:
            return interpolated_value


def interpolate_value(name, config_key, value, section, mapping, schema):
    try:
        properties_schema = schema["definitions"]["service"]["properties"]
        field_def = properties_schema.get(config_key, None)
        return recursive_interpolate(value, mapping, field_def)
    except InvalidInterpolation as e:
        raise ConfigurationError(
            'Invalid interpolation format for "{config_key}" option '
            'in {section} "{name}": "{string}"'.format(
                config_key=config_key,
                name=name,
                section=section,
                string=e.string))


def _get_field_types(field_def):
    if "type" in field_def:
        return field_def["type"]
    if "oneOf" in field_def:
        return [_get_field_types(f_def) for f_def in field_def["oneOf"]]


def _get_sub_field_def(field_def, name):
    if field_def is None:
        return None
    for option_def in field_def.get("oneOf", []):
        if name in option_def.get("properties", []):
            return option_def["properties"][name]
    return field_def.get(name, None)


def recursive_interpolate(obj, mapping, field_def):
    if isinstance(obj, six.string_types):
        value = interpolate(obj, mapping)
        if value != obj and field_def is not None:
            return _cast_interpolated(value, field_def)
        return value
    elif isinstance(obj, dict):
        return dict(
            (key, recursive_interpolate(val, mapping, _get_sub_field_def(field_def, key)))
            for (key, val) in obj.items()
        )
    elif isinstance(obj, list):
        return [recursive_interpolate(val, mapping, _get_sub_field_def(field_def, "items")) for val in obj]
    else:
        return obj


def interpolate(string, mapping):
    if '$' not in string:
        return string
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
