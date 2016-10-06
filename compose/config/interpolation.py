from __future__ import absolute_import
from __future__ import unicode_literals

import logging
from string import Template

import six

from .errors import ConfigurationError
from compose.const import COMPOSEFILE_V1 as V1
from compose.const import COMPOSEFILE_V2_0 as V2_0


log = logging.getLogger(__name__)


class Interpolator(object):

    def __init__(self, templater, mapping):
        self.templater = templater
        self.mapping = mapping

    def interpolate(self, string):
        try:
            return self.templater(string).substitute(self.mapping)
        except ValueError:
            raise InvalidInterpolation(string)


def interpolate_environment_variables(version, config, section, environment):
    if version in (V2_0, V1):
        interpolator = Interpolator(Template, environment)
    else:
        interpolator = Interpolator(TemplateWithDefaults, environment)

    def process_item(name, config_dict):
        return dict(
            (key, interpolate_value(name, key, val, section, interpolator))
            for key, val in (config_dict or {}).items()
        )

    return dict(
        (name, process_item(name, config_dict or {}))
        for name, config_dict in config.items()
    )


def interpolate_value(name, config_key, value, section, interpolator):
    try:
        return recursive_interpolate(value, interpolator)
    except InvalidInterpolation as e:
        raise ConfigurationError(
            'Invalid interpolation format for "{config_key}" option '
            'in {section} "{name}": "{string}"'.format(
                config_key=config_key,
                name=name,
                section=section,
                string=e.string))


def recursive_interpolate(obj, interpolator):
    if isinstance(obj, six.string_types):
        return interpolator.interpolate(obj)
    if isinstance(obj, dict):
        return dict(
            (key, recursive_interpolate(val, interpolator))
            for (key, val) in obj.items()
        )
    if isinstance(obj, list):
        return [recursive_interpolate(val, interpolator) for val in obj]
    return obj


class TemplateWithDefaults(Template):
    idpattern = r'[_a-z][_a-z0-9]*(?::?-[^}]+)?'

    # Modified from python2.7/string.py
    def substitute(self, mapping):
        # Helper function for .sub()
        def convert(mo):
            # Check the most common path first.
            named = mo.group('named') or mo.group('braced')
            if named is not None:
                if ':-' in named:
                    var, _, default = named.partition(':-')
                    return mapping.get(var) or default
                if '-' in named:
                    var, _, default = named.partition('-')
                    return mapping.get(var, default)
                val = mapping[named]
                return '%s' % (val,)
            if mo.group('escaped') is not None:
                return self.delimiter
            if mo.group('invalid') is not None:
                self._invalid(mo)
            raise ValueError('Unrecognized named group in pattern',
                             self.pattern)
        return self.pattern.sub(convert, self.template)


class InvalidInterpolation(Exception):
    def __init__(self, string):
        self.string = string
