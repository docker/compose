import logging
import os
from string import Template

import six

from .errors import ConfigurationError
log = logging.getLogger(__name__)


def interpolate_environment_variables(config):
    mapping = BlankDefaultDict(os.environ)

    return dict(
        (service_name, process_service(service_name, service_dict, mapping))
        for (service_name, service_dict) in config.items()
    )


def process_service(service_name, service_dict, mapping):
    if not isinstance(service_dict, dict):
        raise ConfigurationError(
            'Service "%s" doesn\'t have any configuration options. '
            'All top level keys in your docker-compose.yml must map '
            'to a dictionary of configuration options.' % service_name
        )

    return dict(
        (key, interpolate_value(service_name, key, val, mapping))
        for (key, val) in service_dict.items()
    )


def interpolate_value(service_name, config_key, value, mapping):
    try:
        return recursive_interpolate(value, mapping)
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
                    "The {} variable is not set. Substituting a blank string."
                    .format(key)
                )
                self.missing_keys.append(key)

            return ""


class InvalidInterpolation(Exception):
    def __init__(self, string):
        self.string = string
