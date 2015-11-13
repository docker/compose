"""
Types for objects parsed from the configuration.
"""
from __future__ import absolute_import
from __future__ import unicode_literals

from collections import namedtuple

from compose.config.errors import ConfigurationError


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
