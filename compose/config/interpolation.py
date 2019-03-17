from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import re
from string import Template

import six

from .errors import ConfigurationError
from compose.const import COMPOSEFILE_V2_0 as V2_0
from compose.utils import parse_bytes
from compose.utils import parse_nanoseconds_int


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
    return '{}/{}/{}'.format(section, name, config_key)


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
    except UnsetRequiredSubstitution as e:
        raise ConfigurationError(
            'Missing mandatory value for "{config_key}" option interpolating {value} '
            'in {section} "{name}": {err}'.format(config_key=config_key,
                                                  value=value,
                                                  name=name,
                                                  section=section,
                                                  err=e.err)
        )


def recursive_interpolate(obj, interpolator, config_path):
    def append(config_path, key):
        return '{}/{}'.format(config_path, key)

    if isinstance(obj, six.string_types):
        return converter.convert(config_path, interpolator.interpolate(obj))
    if isinstance(obj, dict):
        return dict(
            (key, recursive_interpolate(val, interpolator, append(config_path, key)))
            for (key, val) in obj.items()
        )
    if isinstance(obj, list):
        return [recursive_interpolate(val, interpolator, config_path) for val in obj]
    return converter.convert(config_path, obj)


class TemplateWithDefaults(Template):
    pattern = r"""
        %(delim)s(?:
            (?P<escaped>%(delim)s) |
            (?P<named>%(id)s)      |
            {(?P<braced>%(bid)s)}  |
            (?P<invalid>)
        )
        """ % {
        'delim': re.escape('$'),
        'id': r'[_a-z][_a-z0-9]*',
        'bid': r'[_a-z][_a-z0-9]*(?:(?P<sep>:?[-?])[^}]*)?',
    }

    @staticmethod
    def process_braced_group(braced, sep, mapping):
        if ':-' == sep:
            var, _, default = braced.partition(':-')
            return mapping.get(var) or default
        elif '-' == sep:
            var, _, default = braced.partition('-')
            return mapping.get(var, default)

        elif ':?' == sep:
            var, _, err = braced.partition(':?')
            result = mapping.get(var)
            if not result:
                raise UnsetRequiredSubstitution(err)
            return result
        elif '?' == sep:
            var, _, err = braced.partition('?')
            if var in mapping:
                return mapping.get(var)
            raise UnsetRequiredSubstitution(err)

    # Modified from python2.7/string.py
    def substitute(self, mapping):
        # Helper function for .sub()

        def convert(mo):
            named = mo.group('named') or mo.group('braced')
            braced = mo.group('braced')
            if braced is not None:
                sep = mo.group('sep')
                if sep:
                    return self.process_braced_group(braced, sep, mapping)

            if named is not None:
                val = mapping[named]
                if isinstance(val, six.binary_type):
                    val = val.decode('utf-8')
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


class UnsetRequiredSubstitution(Exception):
    def __init__(self, custom_err_msg):
        self.err = custom_err_msg


PATH_JOKER = '[^/]+'
FULL_JOKER = '.+'


def re_path(*args):
    return re.compile('^{}$'.format('/'.join(args)))


def re_path_basic(section, name):
    return re_path(section, PATH_JOKER, name)


def service_path(*args):
    return re_path('service', PATH_JOKER, *args)


def to_boolean(s):
    if not isinstance(s, six.string_types):
        return s
    s = s.lower()
    if s in ['y', 'yes', 'true', 'on']:
        return True
    elif s in ['n', 'no', 'false', 'off']:
        return False
    raise ValueError('"{}" is not a valid boolean value'.format(s))


def to_int(s):
    if not isinstance(s, six.string_types):
        return s

    # We must be able to handle octal representation for `mode` values notably
    if six.PY3 and re.match('^0[0-9]+$', s.strip()):
        s = '0o' + s[1:]
    try:
        return int(s, base=0)
    except ValueError:
        raise ValueError('"{}" is not a valid integer'.format(s))


def to_float(s):
    if not isinstance(s, six.string_types):
        return s

    try:
        return float(s)
    except ValueError:
        raise ValueError('"{}" is not a valid float'.format(s))


def to_str(o):
    if isinstance(o, (bool, float, int)):
        return '{}'.format(o)
    return o


def bytes_to_int(s):
    v = parse_bytes(s)
    if v is None:
        raise ValueError('"{}" is not a valid byte value'.format(s))
    return v


def to_microseconds(v):
    if not isinstance(v, six.string_types):
        return v
    return int(parse_nanoseconds_int(v) / 1000)


class ConversionMap(object):
    map = {
        service_path('blkio_config', 'weight'): to_int,
        service_path('blkio_config', 'weight_device', 'weight'): to_int,
        service_path('build', 'labels', FULL_JOKER): to_str,
        service_path('cpus'): to_float,
        service_path('cpu_count'): to_int,
        service_path('cpu_quota'): to_microseconds,
        service_path('cpu_period'): to_microseconds,
        service_path('cpu_rt_period'): to_microseconds,
        service_path('cpu_rt_runtime'): to_microseconds,
        service_path('configs', 'mode'): to_int,
        service_path('secrets', 'mode'): to_int,
        service_path('healthcheck', 'retries'): to_int,
        service_path('healthcheck', 'disable'): to_boolean,
        service_path('deploy', 'labels', PATH_JOKER): to_str,
        service_path('deploy', 'replicas'): to_int,
        service_path('deploy', 'update_config', 'parallelism'): to_int,
        service_path('deploy', 'update_config', 'max_failure_ratio'): to_float,
        service_path('deploy', 'rollback_config', 'parallelism'): to_int,
        service_path('deploy', 'rollback_config', 'max_failure_ratio'): to_float,
        service_path('deploy', 'restart_policy', 'max_attempts'): to_int,
        service_path('mem_swappiness'): to_int,
        service_path('labels', FULL_JOKER): to_str,
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
        service_path('volumes', 'tmpfs', 'size'): bytes_to_int,
        re_path_basic('network', 'attachable'): to_boolean,
        re_path_basic('network', 'external'): to_boolean,
        re_path_basic('network', 'internal'): to_boolean,
        re_path('network', PATH_JOKER, 'labels', FULL_JOKER): to_str,
        re_path_basic('volume', 'external'): to_boolean,
        re_path('volume', PATH_JOKER, 'labels', FULL_JOKER): to_str,
        re_path_basic('secret', 'external'): to_boolean,
        re_path('secret', PATH_JOKER, 'labels', FULL_JOKER): to_str,
        re_path_basic('config', 'external'): to_boolean,
        re_path('config', PATH_JOKER, 'labels', FULL_JOKER): to_str,
    }

    def convert(self, path, value):
        for rexp in self.map.keys():
            if rexp.match(path):
                try:
                    return self.map[rexp](value)
                except ValueError as e:
                    raise ConfigurationError(
                        'Error while attempting to convert {} to appropriate type: {}'.format(
                            path.replace('/', '.'), e
                        )
                    )
        return value


converter = ConversionMap()
