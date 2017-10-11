from __future__ import absolute_import
from __future__ import unicode_literals

import six
import yaml

from compose.config import types
from compose.const import COMPOSEFILE_V1 as V1
from compose.const import COMPOSEFILE_V2_1 as V2_1
from compose.const import COMPOSEFILE_V3_0 as V3_0
from compose.const import COMPOSEFILE_V3_2 as V3_2
from compose.const import COMPOSEFILE_V3_4 as V3_4


def serialize_config_type(dumper, data):
    representer = dumper.represent_str if six.PY3 else dumper.represent_unicode
    return representer(data.repr())


def serialize_dict_type(dumper, data):
    return dumper.represent_dict(data.repr())


def serialize_string(dumper, data):
    """ Ensure boolean-like strings are quoted in the output and escape $ characters """
    representer = dumper.represent_str if six.PY3 else dumper.represent_unicode

    data = data.replace('$', '$$')

    if data.lower() in ('y', 'n', 'yes', 'no', 'on', 'off', 'true', 'false'):
        # Empirically only y/n appears to be an issue, but this might change
        # depending on which PyYaml version is being used. Err on safe side.
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='"')
    return representer(data)


yaml.SafeDumper.add_representer(types.VolumeFromSpec, serialize_config_type)
yaml.SafeDumper.add_representer(types.VolumeSpec, serialize_config_type)
yaml.SafeDumper.add_representer(types.ServiceSecret, serialize_dict_type)
yaml.SafeDumper.add_representer(types.ServiceConfig, serialize_dict_type)
yaml.SafeDumper.add_representer(types.ServicePort, serialize_dict_type)
yaml.SafeDumper.add_representer(str, serialize_string)
yaml.SafeDumper.add_representer(six.text_type, serialize_string)


def denormalize_config(config, image_digests=None):
    result = {'version': str(V2_1) if config.version == V1 else str(config.version)}
    denormalized_services = [
        denormalize_service_dict(
            service_dict,
            config.version,
            image_digests[service_dict['name']] if image_digests else None)
        for service_dict in config.services
    ]
    result['services'] = {
        service_dict.pop('name'): service_dict
        for service_dict in denormalized_services
    }

    for key in ('networks', 'volumes', 'secrets', 'configs'):
        config_dict = getattr(config, key)
        if not config_dict:
            continue
        result[key] = config_dict.copy()
        for name, conf in result[key].items():
            if 'external_name' in conf:
                del conf['external_name']

            if 'name' in conf:
                if config.version < V2_1 or (config.version >= V3_0 and config.version < V3_4):
                    del conf['name']
                elif 'external' in conf:
                    conf['external'] = True

    return result


def serialize_config(config, image_digests=None):
    return yaml.safe_dump(
        denormalize_config(config, image_digests),
        default_flow_style=False,
        indent=2,
        width=80
    )


def serialize_ns_time_value(value):
    result = (value, 'ns')
    table = [
        (1000., 'us'),
        (1000., 'ms'),
        (1000., 's'),
        (60., 'm'),
        (60., 'h')
    ]
    for stage in table:
        tmp = value / stage[0]
        if tmp == int(value / stage[0]):
            value = tmp
            result = (int(value), stage[1])
        else:
            break
    return '{0}{1}'.format(*result)


def denormalize_service_dict(service_dict, version, image_digest=None):
    service_dict = service_dict.copy()

    if image_digest:
        service_dict['image'] = image_digest

    if 'restart' in service_dict:
        service_dict['restart'] = types.serialize_restart_spec(
            service_dict['restart']
        )

    if version == V1 and 'network_mode' not in service_dict:
        service_dict['network_mode'] = 'bridge'

    if 'depends_on' in service_dict and (version < V2_1 or version >= V3_0):
        service_dict['depends_on'] = sorted([
            svc for svc in service_dict['depends_on'].keys()
        ])

    if 'healthcheck' in service_dict:
        if 'interval' in service_dict['healthcheck']:
            service_dict['healthcheck']['interval'] = serialize_ns_time_value(
                service_dict['healthcheck']['interval']
            )
        if 'timeout' in service_dict['healthcheck']:
            service_dict['healthcheck']['timeout'] = serialize_ns_time_value(
                service_dict['healthcheck']['timeout']
            )

        if 'start_period' in service_dict['healthcheck']:
            service_dict['healthcheck']['start_period'] = serialize_ns_time_value(
                service_dict['healthcheck']['start_period']
            )
    if 'ports' in service_dict and version < V3_2:
        service_dict['ports'] = [
            p.legacy_repr() if isinstance(p, types.ServicePort) else p
            for p in service_dict['ports']
        ]

    return service_dict
