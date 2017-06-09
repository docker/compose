"""
Types for objects parsed from the configuration.
"""
from __future__ import absolute_import
from __future__ import unicode_literals

import os
import re
from collections import namedtuple

import six
from docker.utils.ports import build_port_bindings

from ..const import COMPOSEFILE_V1 as V1
from .errors import ConfigurationError
from compose.const import IS_WINDOWS_PLATFORM
from compose.utils import splitdrive

win32_root_path_pattern = re.compile(r'^[A-Za-z]\:\\.*')


class VolumeFromSpec(namedtuple('_VolumeFromSpec', 'source mode type')):

    # TODO: drop service_names arg when v1 is removed
    @classmethod
    def parse(cls, volume_from_config, service_names, version):
        func = cls.parse_v1 if version == V1 else cls.parse_v2
        return func(service_names, volume_from_config)

    @classmethod
    def parse_v1(cls, service_names, volume_from_config):
        parts = volume_from_config.split(':')
        if len(parts) > 2:
            raise ConfigurationError(
                "volume_from {} has incorrect format, should be "
                "service[:mode]".format(volume_from_config))

        if len(parts) == 1:
            source = parts[0]
            mode = 'rw'
        else:
            source, mode = parts

        type = 'service' if source in service_names else 'container'
        return cls(source, mode, type)

    @classmethod
    def parse_v2(cls, service_names, volume_from_config):
        parts = volume_from_config.split(':')
        if len(parts) > 3:
            raise ConfigurationError(
                "volume_from {} has incorrect format, should be one of "
                "'<service name>[:<mode>]' or "
                "'container:<container name>[:<mode>]'".format(volume_from_config))

        if len(parts) == 1:
            source = parts[0]
            return cls(source, 'rw', 'service')

        if len(parts) == 2:
            if parts[0] == 'container':
                type, source = parts
                return cls(source, 'rw', type)

            source, mode = parts
            return cls(source, mode, 'service')

        if len(parts) == 3:
            type, source, mode = parts
            if type not in ('service', 'container'):
                raise ConfigurationError(
                    "Unknown volumes_from type '{}' in '{}'".format(
                        type,
                        volume_from_config))

        return cls(source, mode, type)

    def repr(self):
        return '{v.type}:{v.source}:{v.mode}'.format(v=self)


def parse_restart_spec(restart_config):
    if not restart_config:
        return None
    parts = restart_config.split(':')
    if len(parts) > 2:
        raise ConfigurationError(
            "Restart %s has incorrect format, should be "
            "mode[:max_retry]" % restart_config)
    if len(parts) == 2:
        name, max_retry_count = parts
    else:
        name, = parts
        max_retry_count = 0

    return {'Name': name, 'MaximumRetryCount': int(max_retry_count)}


def serialize_restart_spec(restart_spec):
    if not restart_spec:
        return ''
    parts = [restart_spec['Name']]
    if restart_spec['MaximumRetryCount']:
        parts.append(six.text_type(restart_spec['MaximumRetryCount']))
    return ':'.join(parts)


def parse_extra_hosts(extra_hosts_config):
    if not extra_hosts_config:
        return {}

    if isinstance(extra_hosts_config, dict):
        return dict(extra_hosts_config)

    if isinstance(extra_hosts_config, list):
        extra_hosts_dict = {}
        for extra_hosts_line in extra_hosts_config:
            # TODO: validate string contains ':' ?
            host, ip = extra_hosts_line.split(':', 1)
            extra_hosts_dict[host.strip()] = ip.strip()
        return extra_hosts_dict


def normalize_path_for_engine(path):
    """Windows paths, c:\my\path\shiny, need to be changed to be compatible with
    the Engine. Volume paths are expected to be linux style /c/my/path/shiny/
    """
    drive, tail = splitdrive(path)

    if drive:
        path = '/' + drive.lower().rstrip(':') + tail

    return path.replace('\\', '/')


class VolumeSpec(namedtuple('_VolumeSpec', 'external internal mode')):

    @classmethod
    def _parse_unix(cls, volume_config):
        parts = volume_config.split(':')

        if len(parts) > 3:
            raise ConfigurationError(
                "Volume %s has incorrect format, should be "
                "external:internal[:mode]" % volume_config)

        if len(parts) == 1:
            external = None
            internal = os.path.normpath(parts[0])
        else:
            external = os.path.normpath(parts[0])
            internal = os.path.normpath(parts[1])

        mode = 'rw'
        if len(parts) == 3:
            mode = parts[2]

        return cls(external, internal, mode)

    @classmethod
    def _parse_win32(cls, volume_config, normalize):
        # relative paths in windows expand to include the drive, eg C:\
        # so we join the first 2 parts back together to count as one
        mode = 'rw'

        def separate_next_section(volume_config):
            drive, tail = splitdrive(volume_config)
            parts = tail.split(':', 1)
            if drive:
                parts[0] = drive + parts[0]
            return parts

        parts = separate_next_section(volume_config)
        if len(parts) == 1:
            internal = parts[0]
            external = None
        else:
            external = parts[0]
            parts = separate_next_section(parts[1])
            external = os.path.normpath(external)
            internal = parts[0]
            if len(parts) > 1:
                if ':' in parts[1]:
                    raise ConfigurationError(
                        "Volume %s has incorrect format, should be "
                        "external:internal[:mode]" % volume_config
                    )
                mode = parts[1]

        if normalize:
            external = normalize_path_for_engine(external) if external else None

        return cls(external, internal, mode)

    @classmethod
    def parse(cls, volume_config, normalize=False):
        """Parse a volume_config path and split it into external:internal[:mode]
        parts to be returned as a valid VolumeSpec.
        """
        if IS_WINDOWS_PLATFORM:
            return cls._parse_win32(volume_config, normalize)
        else:
            return cls._parse_unix(volume_config)

    def repr(self):
        external = self.external + ':' if self.external else ''
        mode = ':' + self.mode if self.external else ''
        return '{ext}{v.internal}{mode}'.format(mode=mode, ext=external, v=self)

    @property
    def is_named_volume(self):
        res = self.external and not self.external.startswith(('.', '/', '~'))
        if not IS_WINDOWS_PLATFORM:
            return res

        return (
            res and not self.external.startswith('\\') and
            not win32_root_path_pattern.match(self.external)
        )


class ServiceLink(namedtuple('_ServiceLink', 'target alias')):

    @classmethod
    def parse(cls, link_spec):
        target, _, alias = link_spec.partition(':')
        if not alias:
            alias = target
        return cls(target, alias)

    def repr(self):
        if self.target == self.alias:
            return self.target
        return '{s.target}:{s.alias}'.format(s=self)

    @property
    def merge_field(self):
        return self.alias


class ServiceConfigBase(namedtuple('_ServiceConfigBase', 'source target uid gid mode')):
    @classmethod
    def parse(cls, spec):
        if isinstance(spec, six.string_types):
            return cls(spec, None, None, None, None)
        return cls(
            spec.get('source'),
            spec.get('target'),
            spec.get('uid'),
            spec.get('gid'),
            spec.get('mode'),
        )

    @property
    def merge_field(self):
        return self.source

    def repr(self):
        return dict(
            [(k, v) for k, v in zip(self._fields, self) if v is not None]
        )


class ServiceSecret(ServiceConfigBase):
    pass


class ServiceConfig(ServiceConfigBase):
    pass


class ServicePort(namedtuple('_ServicePort', 'target published protocol mode external_ip')):
    def __new__(cls, target, published, *args, **kwargs):
        try:
            if target:
                target = int(target)
        except ValueError:
            raise ConfigurationError('Invalid target port: {}'.format(target))

        try:
            if published:
                published = int(published)
        except ValueError:
            raise ConfigurationError('Invalid published port: {}'.format(published))

        return super(ServicePort, cls).__new__(
            cls, target, published, *args, **kwargs
        )

    @classmethod
    def parse(cls, spec):
        if isinstance(spec, cls):
            # When extending a service with ports, the port definitions have already been parsed
            return [spec]

        if not isinstance(spec, dict):
            result = []
            for k, v in build_port_bindings([spec]).items():
                if '/' in k:
                    target, proto = k.split('/', 1)
                else:
                    target, proto = (k, None)
                for pub in v:
                    if pub is None:
                        result.append(
                            cls(target, None, proto, None, None)
                        )
                    elif isinstance(pub, tuple):
                        result.append(
                            cls(target, pub[1], proto, None, pub[0])
                        )
                    else:
                        result.append(
                            cls(target, pub, proto, None, None)
                        )
            return result

        return [cls(
            spec.get('target'),
            spec.get('published'),
            spec.get('protocol'),
            spec.get('mode'),
            None
        )]

    @property
    def merge_field(self):
        return (self.target, self.published, self.external_ip, self.protocol)

    def repr(self):
        return dict(
            [(k, v) for k, v in zip(self._fields, self) if v is not None]
        )

    def legacy_repr(self):
        return normalize_port_dict(self.repr())


def normalize_port_dict(port):
    return '{external_ip}{has_ext_ip}{published}{is_pub}{target}/{protocol}'.format(
        published=port.get('published', ''),
        is_pub=(':' if port.get('published') or port.get('external_ip') else ''),
        target=port.get('target'),
        protocol=port.get('protocol', 'tcp'),
        external_ip=port.get('external_ip', ''),
        has_ext_ip=(':' if port.get('external_ip') else ''),
    )
