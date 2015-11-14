"""
Types for objects parsed from the configuration.
"""
from __future__ import absolute_import
from __future__ import unicode_literals

import os
from collections import namedtuple

from compose.config.errors import ConfigurationError
from compose.const import IS_WINDOWS_PLATFORM


class VolumeFromSpec(namedtuple('_VolumeFromSpec', 'source mode')):

    @classmethod
    def parse(cls, volume_from_config):
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

        return cls(source, mode)


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


def parse_extra_hosts(extra_hosts_config):
    if not extra_hosts_config:
        return {}

    if isinstance(extra_hosts_config, dict):
        return dict(extra_hosts_config)

    if isinstance(extra_hosts_config, list):
        extra_hosts_dict = {}
        for extra_hosts_line in extra_hosts_config:
            # TODO: validate string contains ':' ?
            host, ip = extra_hosts_line.split(':')
            extra_hosts_dict[host.strip()] = ip.strip()
        return extra_hosts_dict


def normalize_paths_for_engine(external_path, internal_path):
    """Windows paths, c:\my\path\shiny, need to be changed to be compatible with
    the Engine. Volume paths are expected to be linux style /c/my/path/shiny/
    """
    if not IS_WINDOWS_PLATFORM:
        return external_path, internal_path

    if external_path:
        drive, tail = os.path.splitdrive(external_path)

        if drive:
            external_path = '/' + drive.lower().rstrip(':') + tail

        external_path = external_path.replace('\\', '/')

    return external_path, internal_path.replace('\\', '/')


class VolumeSpec(namedtuple('_VolumeSpec', 'external internal mode')):

    @classmethod
    def parse(cls, volume_config):
        """Parse a volume_config path and split it into external:internal[:mode]
        parts to be returned as a valid VolumeSpec.
        """
        if IS_WINDOWS_PLATFORM:
            # relative paths in windows expand to include the drive, eg C:\
            # so we join the first 2 parts back together to count as one
            drive, tail = os.path.splitdrive(volume_config)
            parts = tail.split(":")

            if drive:
                parts[0] = drive + parts[0]
        else:
            parts = volume_config.split(':')

        if len(parts) > 3:
            raise ConfigurationError(
                "Volume %s has incorrect format, should be "
                "external:internal[:mode]" % volume_config)

        if len(parts) == 1:
            external, internal = normalize_paths_for_engine(
                None,
                os.path.normpath(parts[0]))
        else:
            external, internal = normalize_paths_for_engine(
                os.path.normpath(parts[0]),
                os.path.normpath(parts[1]))

        mode = 'rw'
        if len(parts) == 3:
            mode = parts[2]

        return cls(external, internal, mode)
