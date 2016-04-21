from __future__ import absolute_import
from __future__ import unicode_literals

import logging
from string import Template

import six

from .errors import ConfigurationError
from .func_map import func_map
from .func_map import func_regexp
from .func_map import inhibate_double_arobase
from .func_map import InvalidHelperFunction

log = logging.getLogger(__name__)


def interpolate_environment_variables(config, section, environment):

    def process_item(name, config_dict):
        return dict(
            (key, interpolate_value(name, key, val, section, environment))
            for key, val in (config_dict or {}).items()
        )

    return dict(
        (name, process_item(name, config_dict or {}))
        for name, config_dict in config.items()
    )


def interpolate_value(name, config_key, value, section, mapping):
    try:
        return recursive_interpolate(value, mapping)
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
        return interpolate(obj, mapping)
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
    string = interpolate_function(string)
    try:
        return Template(string).substitute(mapping)
    except ValueError:
        raise InvalidInterpolation(string)


def interpolate_function(string):
    for match in func_regexp.finditer(string):
        if string[:2] == '@@':
            continue
        cmd = match.group(1)
        if cmd in func_map:
            string = string.replace(match.group(0), str(func_map[cmd]()))
        else:
            raise InvalidHelperFunction('Unkwown helper function "%s"' % cmd)
    string = inhibate_double_arobase.sub(lambda m: r'@{%s}' % m.group(1), string)
    return string


class InvalidInterpolation(Exception):
    def __init__(self, string):
        self.string = string
