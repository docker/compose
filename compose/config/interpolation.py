from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import re
from string import Template
from subprocess import CalledProcessError
from subprocess import check_output

import six

from .errors import ConfigurationError

log = logging.getLogger(__name__)
command_regexp = re.compile('\$\(\((.*?)\)\)')


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
    try:
        return Template(string).substitute(mapping)
    except ValueError:
        has_command = False
        for matches in command_regexp.finditer(string):
            has_command = True
            string = interpolate_command(string, matches.group(1), matches.group(0))
        if has_command:
            return interpolate(string, mapping)
        else:
            raise InvalidInterpolation(string)


def interpolate_command(string, command, base):
    try:
        m = check_output(command, shell=True).strip()
        string = string.replace(base, m.decode())
    except CalledProcessError as e:
        raise ConfigurationError(
            '"{cmd}" command returned {ret} exit status.\n'
            'Command output is:\n'
            '{out}\n'.format(
                cmd=e.cmd,
                ret=e.returncode,
                out=e.output.decode()))
    except Exception as e:
        raise InvalidInterpolation(string)
    return string


class InvalidInterpolation(Exception):
    def __init__(self, string):
        self.string = string
