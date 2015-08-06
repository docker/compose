import os
from string import Template

import six

from .errors import ConfigurationError

import logging
log = logging.getLogger(__name__)


def interpolate_environment_variables(config):
    return dict(
        (service_name, process_service(service_name, service_dict))
        for (service_name, service_dict) in config.items()
    )


def process_service(service_name, service_dict):
    if not isinstance(service_dict, dict):
        raise ConfigurationError(
            'Service "%s" doesn\'t have any configuration options. '
            'All top level keys in your docker-compose.yml must map '
            'to a dictionary of configuration options.' % service_name
        )

    return dict(
        (key, interpolate_value(service_name, key, val))
        for (key, val) in service_dict.items()
    )


def interpolate_value(service_name, config_key, value):
    try:
        return recursive_interpolate(value)
    except InvalidInterpolation as e:
        raise ConfigurationError(
            'Invalid interpolation format for "{config_key}" option '
            'in service "{service_name}": "{string}"'
            .format(
                config_key=config_key,
                service_name=service_name,
                string=e.string,
            )
        )


def recursive_interpolate(obj):
    if isinstance(obj, six.string_types):
        return interpolate(obj, os.environ)
    elif isinstance(obj, dict):
        return dict(
            (key, recursive_interpolate(val))
            for (key, val) in obj.items()
        )
    elif isinstance(obj, list):
        return map(recursive_interpolate, obj)
    else:
        return obj


def interpolate(string, mapping):
    try:
        return Template(string).substitute(BlankDefaultDict(mapping))
    except ValueError:
        raise InvalidInterpolation(string)


class BlankDefaultDict(dict):
    def __init__(self, mapping):
        super(BlankDefaultDict, self).__init__(mapping)

    def __getitem__(self, key):
        try:
            return super(BlankDefaultDict, self).__getitem__(key)
        except KeyError:
            log.warn(
                "The {} variable is not set. Substituting a blank string."
                .format(key)
            )
            return ""


class InvalidInterpolation(Exception):
    def __init__(self, string):
        self.string = string
