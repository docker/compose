import yaml

from compose.config import types
from compose.const import COMPOSE_SPEC as VERSION
from compose.const import COMPOSEFILE_V1 as V1


def serialize_config_type(dumper, data):
    representer = dumper.represent_str
    return representer(data.repr())


def serialize_dict_type(dumper, data):
    return dumper.represent_dict(data.repr())


def serialize_string(dumper, data):
    """ Ensure boolean-like strings are quoted in the output """
    representer = dumper.represent_str

    if isinstance(data, bytes):
        data = data.decode('utf-8')

    if data.lower() in ('y', 'n', 'yes', 'no', 'on', 'off', 'true', 'false'):
        # Empirically only y/n appears to be an issue, but this might change
        # depending on which PyYaml version is being used. Err on safe side.
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='"')
    return representer(data)


def serialize_string_escape_dollar(dumper, data):
    """ Ensure boolean-like strings are quoted in the output and escape $ characters """
    data = data.replace('$', '$$')
    return serialize_string(dumper, data)


yaml.SafeDumper.add_representer(types.MountSpec, serialize_dict_type)
yaml.SafeDumper.add_representer(types.VolumeFromSpec, serialize_config_type)
yaml.SafeDumper.add_representer(types.VolumeSpec, serialize_config_type)
yaml.SafeDumper.add_representer(types.SecurityOpt, serialize_config_type)
yaml.SafeDumper.add_representer(types.ServiceSecret, serialize_dict_type)
yaml.SafeDumper.add_representer(types.ServiceConfig, serialize_dict_type)
yaml.SafeDumper.add_representer(types.ServicePort, serialize_dict_type)


def denormalize_config(config, image_digests=None):
    result = {'version': str(config.config_version)}
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
                if 'external' in conf:
                    conf['external'] = bool(conf['external'])
    return result


def serialize_config(config, image_digests=None, escape_dollar=True):
    if escape_dollar:
        yaml.SafeDumper.add_representer(str, serialize_string_escape_dollar)
        yaml.SafeDumper.add_representer(str, serialize_string_escape_dollar)
    else:
        yaml.SafeDumper.add_representer(str, serialize_string)
        yaml.SafeDumper.add_representer(str, serialize_string)
    return yaml.safe_dump(
        denormalize_config(config, image_digests),
        default_flow_style=False,
        indent=2,
        width=80,
        allow_unicode=True
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
    return '{}{}'.format(*result)


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

    if 'ports' in service_dict:
        service_dict['ports'] = [
            p.legacy_repr() if p.external_ip or version < VERSION else p
            for p in service_dict['ports']
        ]
    if 'volumes' in service_dict and (version == V1):
        service_dict['volumes'] = [
            v.legacy_repr() if isinstance(v, types.MountSpec) else v for v in service_dict['volumes']
        ]

    return service_dict
