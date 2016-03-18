from __future__ import absolute_import
from __future__ import unicode_literals

import six
import yaml

from compose.config import types
from compose.config.config import V1
from compose.config.config import V2_0


def serialize_config_type(dumper, data):
    representer = dumper.represent_str if six.PY3 else dumper.represent_unicode
    return representer(data.repr())


yaml.SafeDumper.add_representer(types.VolumeFromSpec, serialize_config_type)
yaml.SafeDumper.add_representer(types.VolumeSpec, serialize_config_type)


def serialize_config(config):
    services = {service.pop('name'): service for service in config.services}

    if config.version == V1:
        for service_dict in services.values():
            if 'network_mode' not in service_dict:
                service_dict['network_mode'] = 'bridge'

    output = {
        'version': V2_0,
        'services': services,
        'networks': config.networks,
        'volumes': config.volumes,
    }

    return yaml.safe_dump(
        output,
        default_flow_style=False,
        indent=2,
        width=80)
