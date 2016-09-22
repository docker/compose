from __future__ import absolute_import
from __future__ import unicode_literals

import six
import yaml

from compose.config import types
from compose.config.config import V1
from compose.config.config import V2_0
from compose.config.config import V2_1


def serialize_config_type(dumper, data):
    representer = dumper.represent_str if six.PY3 else dumper.represent_unicode
    return representer(data.repr())


yaml.SafeDumper.add_representer(types.VolumeFromSpec, serialize_config_type)
yaml.SafeDumper.add_representer(types.VolumeSpec, serialize_config_type)


def denormalize_config(config):
    denormalized_services = [
        denormalize_service_dict(service_dict, config.version)
        for service_dict in config.services
    ]
    services = {
        service_dict.pop('name'): service_dict
        for service_dict in denormalized_services
    }
    networks = config.networks.copy()
    for net_name, net_conf in networks.items():
        if 'external_name' in net_conf:
            del net_conf['external_name']

    version = config.version
    if version not in (V2_0, V2_1):
        version = V2_1

    return {
        'version': version,
        'services': services,
        'networks': networks,
        'volumes': config.volumes,
    }


def serialize_config(config):
    return yaml.safe_dump(
        denormalize_config(config),
        default_flow_style=False,
        indent=2,
        width=80)


def denormalize_service_dict(service_dict, version):
    service_dict = service_dict.copy()

    if 'restart' in service_dict:
        service_dict['restart'] = types.serialize_restart_spec(service_dict['restart'])

    if version == V1 and 'network_mode' not in service_dict:
        service_dict['network_mode'] = 'bridge'

    return service_dict
