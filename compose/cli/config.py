from __future__ import unicode_literals
from __future__ import absolute_import

import six
import errno
import yaml
import os
import re

from compose.cli import errors


def resolve_environment_vars(value):
    """
    Matches our environment variable pattern, replaces the value with the value in the env
    Supports multiple variables per line

    A future improvement here would be to also reference a separate dictionary that keeps track of
    configurable variables via flat file.

    :param value: any value that is reachable by a key in a dict represented by a yaml file
    :return: the value itself if not a string with an environment variable, otherwise the value specified in the env
    """
    if not isinstance(value, six.string_types):
        return value

    # First, identify any variables
    env_regex = re.compile(r'([^\\])?(?P<variable>\$\{[^\}]+\})')

    split_string = re.split(env_regex, value)
    result_string = ''

    instance_regex = r'^\$\{(?P<env_var>[^\}^:]+)(:(?P<default_val>[^\}]+))?\}$'
    for split_value in split_string:
        if not split_value:
            continue

        match_object = re.match(instance_regex, split_value)
        if not match_object:
            result_string += split_value
            continue

        result = os.environ.get(match_object.group('env_var'), match_object.group('default_val'))

        if result is None:
            raise errors.UserError("No value for ${%s} found in environment." % (match_object.group('env_var'))
                                   + "Please set a value in the environment or provide a default.")

        result_string += re.sub(instance_regex, result, split_value)

    return result_string


def with_environment_vars(value):
    """
    Recursively interpolates environment variables for a structured or unstructured value

    :param value: a dict, list, or any other kind of value

    :return: the dict with its values interpolated from the env
    :rtype: dict
    """
    if type(value) == dict:
        return dict([(subkey, with_environment_vars(subvalue))
                     for subkey, subvalue in value.items()])
    elif type(value) == list:
        return [resolve_environment_vars(x) for x in value]
    else:
        return resolve_environment_vars(value)


def from_yaml_with_environment_vars(yaml_filename):
    """
    Resolves environment variables in values defined by a YAML file and transformed into a dict
    :param yaml_filename: the name of the yaml file
    :type yaml_filename: str

    :return: a dict with environment variables properly interpolated
    """
    try:
        with open(yaml_filename, 'r') as fh:
            return with_environment_vars(yaml.safe_load(fh))
    except IOError as e:
        if e.errno == errno.ENOENT:
            raise errors.FigFileNotFound(os.path.basename(e.filename))
        raise errors.UserError(six.text_type(e))
