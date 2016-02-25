from __future__ import absolute_import
from __future__ import unicode_literals

import ruamel.yaml
import six

from compose.config import types


def serialize_config_type(dumper, data):
    representer = dumper.represent_str if six.PY3 else dumper.represent_unicode
    return representer(data.repr())


ruamel.yaml.SafeDumper.add_representer(types.VolumeFromSpec, serialize_config_type)
ruamel.yaml.SafeDumper.add_representer(types.VolumeSpec, serialize_config_type)


def serialize_config(config):
    output = {
        'version': config.version,
        'services': {service.pop('name'): service for service in config.services},
        'networks': config.networks,
        'volumes': config.volumes,
    }
    return ruamel.yaml.safe_dump(
        output,
        default_flow_style=False,
        indent=2,
        width=80)
