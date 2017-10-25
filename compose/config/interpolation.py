from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import re
from string import Template

import six

from .errors import ConfigurationError
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
    if version <= V2_0:
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


def get_config_path(config_key, section, name):
    return '{}.{}.{}'.format(section, name, config_key)


def interpolate_value(name, config_key, value, section, interpolator):
    try:
        return recursive_interpolate(value, interpolator, get_config_path(config_key, section, name))
    except InvalidInterpolation as e:
        raise ConfigurationError(
            'Invalid interpolation format for "{config_key}" option '
            'in {section} "{name}": "{string}"'.format(
                config_key=config_key,
                name=name,
                section=section,
                string=e.string))


def recursive_interpolate(obj, interpolator, config_path):
    def append(config_path, key):
        return '{}.{}'.format(config_path, key)

    if isinstance(obj, six.string_types):
        return converter.convert(config_path, interpolator.interpolate(obj))
    if isinstance(obj, dict):
        return dict(
            (key, recursive_interpolate(val, interpolator, append(config_path, key)))
            for (key, val) in obj.items()
        )
    if isinstance(obj, list):
        return [recursive_interpolate(val, interpolator, config_path) for val in obj]
    return obj


class TemplateWithDefaults(Template):
    idpattern = r'[_a-z][_a-z0-9]*(?::?-[^}]*)?'

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


PATH_JOKER = '[^.]+'


def re_path(*args):
    return re.compile('^{}$'.format('.'.join(args)))


def re_path_basic(section, name):
    return re_path(section, PATH_JOKER, name)


def service_path(*args):
    return re_path('service', PATH_JOKER, *args)


def to_boolean(s):
    s = s.lower()
    if s in ['y', 'yes', 'true', 'on']:
        return True
    elif s in ['n', 'no', 'false', 'off']:
        return False
    raise ValueError('"{}" is not a valid boolean value'.format(s))


def to_int(s):
    # We must be able to handle octal representation for `mode` values notably
    if six.PY3 and re.match('^0[0-9]+$', s.strip()):
        s = '0o' + s[1:]
    return int(s, base=0)


class ConversionMap(object):
    map = {
        service_path('blkio_config', 'weight'): to_int,
        service_path('blkio_config', 'weight_device', 'weight'): to_int,
        service_path('cpus'): float,
        service_path('cpu_count'): to_int,
        service_path('configs', 'mode'): to_int,
        service_path('secrets', 'mode'): to_int,
        service_path('healthcheck', 'retries'): to_int,
        service_path('healthcheck', 'disable'): to_boolean,
        service_path('deploy', 'replicas'): to_int,
        service_path('deploy', 'update_config', 'parallelism'): to_int,
        service_path('deploy', 'update_config', 'max_failure_ratio'): float,
        service_path('deploy', 'restart_policy', 'max_attempts'): to_int,
        service_path('mem_swappiness'): to_int,
        service_path('oom_kill_disable'): to_boolean,
        service_path('oom_score_adj'): to_int,
        service_path('ports', 'target'): to_int,
        service_path('ports', 'published'): to_int,
        service_path('scale'): to_int,
        service_path('ulimits', PATH_JOKER): to_int,
        service_path('ulimits', PATH_JOKER, 'soft'): to_int,
        service_path('ulimits', PATH_JOKER, 'hard'): to_int,
        service_path('privileged'): to_boolean,
        service_path('read_only'): to_boolean,
        service_path('stdin_open'): to_boolean,
        service_path('tty'): to_boolean,
        service_path('volumes', 'read_only'): to_boolean,
        service_path('volumes', 'volume', 'nocopy'): to_boolean,
        re_path_basic('network', 'attachable'): to_boolean,
        re_path_basic('network', 'external'): to_boolean,
        re_path_basic('network', 'internal'): to_boolean,
        re_path_basic('volume', 'external'): to_boolean,
        re_path_basic('secret', 'external'): to_boolean,
        re_path_basic('config', 'external'): to_boolean,
    }

    def convert(self, path, value):
        for rexp in self.map.keys():
            if rexp.match(path):
                return self.map[rexp](value)
        return value


converter = ConversionMap()
