from __future__ import absolute_import
from __future__ import unicode_literals

import json
import logging
import os
import re
import sys

import six
from docker.utils.ports import split_port
from jsonschema import Draft4Validator
from jsonschema import FormatChecker
from jsonschema import RefResolver
from jsonschema import ValidationError

from ..const import COMPOSEFILE_V1 as V1
from ..const import NANOCPUS_SCALE
from .errors import ConfigurationError
from .errors import VERSION_EXPLANATION
from .sort_services import get_service_name_from_network_mode


log = logging.getLogger(__name__)


DOCKER_CONFIG_HINTS = {
    'cpu_share': 'cpu_shares',
    'add_host': 'extra_hosts',
    'hosts': 'extra_hosts',
    'extra_host': 'extra_hosts',
    'device': 'devices',
    'link': 'links',
    'memory_swap': 'memswap_limit',
    'port': 'ports',
    'privilege': 'privileged',
    'priviliged': 'privileged',
    'privilige': 'privileged',
    'volume': 'volumes',
    'workdir': 'working_dir',
}


VALID_NAME_CHARS = '[a-zA-Z0-9\._\-]'
VALID_EXPOSE_FORMAT = r'^\d+(\-\d+)?(\/[a-zA-Z]+)?$'

VALID_IPV4_SEG = r'(\d{1,2}|1\d{2}|2[0-4]\d|25[0-5])'
VALID_IPV4_ADDR = "({IPV4_SEG}\.){{3}}{IPV4_SEG}".format(IPV4_SEG=VALID_IPV4_SEG)
VALID_REGEX_IPV4_CIDR = "^{IPV4_ADDR}/(\d|[1-2]\d|3[0-2])$".format(IPV4_ADDR=VALID_IPV4_ADDR)

VALID_IPV6_SEG = r'[0-9a-fA-F]{1,4}'
VALID_REGEX_IPV6_CIDR = "".join("""
^
(
    (({IPV6_SEG}:){{7}}{IPV6_SEG})|
    (({IPV6_SEG}:){{1,7}}:)|
    (({IPV6_SEG}:){{1,6}}(:{IPV6_SEG}){{1,1}})|
    (({IPV6_SEG}:){{1,5}}(:{IPV6_SEG}){{1,2}})|
    (({IPV6_SEG}:){{1,4}}(:{IPV6_SEG}){{1,3}})|
    (({IPV6_SEG}:){{1,3}}(:{IPV6_SEG}){{1,4}})|
    (({IPV6_SEG}:){{1,2}}(:{IPV6_SEG}){{1,5}})|
    (({IPV6_SEG}:){{1,1}}(:{IPV6_SEG}){{1,6}})|
    (:((:{IPV6_SEG}){{1,7}}|:))|
    (fe80:(:{IPV6_SEG}){{0,4}}%[0-9a-zA-Z]{{1,}})|
    (::(ffff(:0{{1,4}}){{0,1}}:){{0,1}}{IPV4_ADDR})|
    (({IPV6_SEG}:){{1,4}}:{IPV4_ADDR})
)
/(\d|[1-9]\d|1[0-1]\d|12[0-8])
$
""".format(IPV6_SEG=VALID_IPV6_SEG, IPV4_ADDR=VALID_IPV4_ADDR).split())


@FormatChecker.cls_checks(format="ports", raises=ValidationError)
def format_ports(instance):
    try:
        split_port(instance)
    except ValueError as e:
        raise ValidationError(six.text_type(e))
    return True


@FormatChecker.cls_checks(format="expose", raises=ValidationError)
def format_expose(instance):
    if isinstance(instance, six.string_types):
        if not re.match(VALID_EXPOSE_FORMAT, instance):
            raise ValidationError(
                "should be of the format 'PORT[/PROTOCOL]'")

    return True


@FormatChecker.cls_checks("subnet_ip_address", raises=ValidationError)
def format_subnet_ip_address(instance):
    if isinstance(instance, six.string_types):
        if not re.match(VALID_REGEX_IPV4_CIDR, instance) and \
                not re.match(VALID_REGEX_IPV6_CIDR, instance):
            raise ValidationError("should use the CIDR format")

    return True


def match_named_volumes(service_dict, project_volumes):
    service_volumes = service_dict.get('volumes', [])
    for volume_spec in service_volumes:
        if volume_spec.is_named_volume and volume_spec.external not in project_volumes:
            raise ConfigurationError(
                'Named volume "{0}" is used in service "{1}" but no'
                ' declaration was found in the volumes section.'.format(
                    volume_spec.repr(), service_dict.get('name')
                )
            )


def python_type_to_yaml_type(type_):
    type_name = type(type_).__name__
    return {
        'dict': 'mapping',
        'list': 'array',
        'int': 'number',
        'float': 'number',
        'bool': 'boolean',
        'unicode': 'string',
        'str': 'string',
        'bytes': 'string',
    }.get(type_name, type_name)


def validate_config_section(filename, config, section):
    """Validate the structure of a configuration section. This must be done
    before interpolation so it's separate from schema validation.
    """
    if not isinstance(config, dict):
        raise ConfigurationError(
            "In file '{filename}', {section} must be a mapping, not "
            "{type}.".format(
                filename=filename,
                section=section,
                type=anglicize_json_type(python_type_to_yaml_type(config))))

    for key, value in config.items():
        if not isinstance(key, six.string_types):
            raise ConfigurationError(
                "In file '{filename}', the {section} name {name} must be a "
                "quoted string, i.e. '{name}'.".format(
                    filename=filename,
                    section=section,
                    name=key))

        if not isinstance(value, (dict, type(None))):
            raise ConfigurationError(
                "In file '{filename}', {section} '{name}' must be a mapping not "
                "{type}.".format(
                    filename=filename,
                    section=section,
                    name=key,
                    type=anglicize_json_type(python_type_to_yaml_type(value))))


def validate_top_level_object(config_file):
    if not isinstance(config_file.config, dict):
        raise ConfigurationError(
            "Top level object in '{}' needs to be an object not '{}'.".format(
                config_file.filename,
                type(config_file.config)))


def validate_ulimits(service_config):
    ulimit_config = service_config.config.get('ulimits', {})
    for limit_name, soft_hard_values in six.iteritems(ulimit_config):
        if isinstance(soft_hard_values, dict):
            if not soft_hard_values['soft'] <= soft_hard_values['hard']:
                raise ConfigurationError(
                    "Service '{s.name}' has invalid ulimit '{ulimit}'. "
                    "'soft' value can not be greater than 'hard' value ".format(
                        s=service_config,
                        ulimit=ulimit_config))


def validate_extends_file_path(service_name, extends_options, filename):
    """
    The service to be extended must either be defined in the config key 'file',
    or within 'filename'.
    """
    error_prefix = "Invalid 'extends' configuration for %s:" % service_name

    if 'file' not in extends_options and filename is None:
        raise ConfigurationError(
            "%s you need to specify a 'file', e.g. 'file: something.yml'" % error_prefix
        )


def validate_network_mode(service_config, service_names):
    network_mode = service_config.config.get('network_mode')
    if not network_mode:
        return

    if 'networks' in service_config.config:
        raise ConfigurationError("'network_mode' and 'networks' cannot be combined")

    dependency = get_service_name_from_network_mode(network_mode)
    if not dependency:
        return

    if dependency not in service_names:
        raise ConfigurationError(
            "Service '{s.name}' uses the network stack of service '{dep}' which "
            "is undefined.".format(s=service_config, dep=dependency))


def validate_pid_mode(service_config, service_names):
    pid_mode = service_config.config.get('pid')
    if not pid_mode:
        return

    dependency = get_service_name_from_network_mode(pid_mode)
    if not dependency:
        return
    if dependency not in service_names:
        raise ConfigurationError(
            "Service '{s.name}' uses the PID namespace of service '{dep}' which "
            "is undefined.".format(s=service_config, dep=dependency)
        )


def validate_links(service_config, service_names):
    for link in service_config.config.get('links', []):
        if link.split(':')[0] not in service_names:
            raise ConfigurationError(
                "Service '{s.name}' has a link to service '{link}' which is "
                "undefined.".format(s=service_config, link=link))


def validate_depends_on(service_config, service_names):
    deps = service_config.config.get('depends_on', {})
    for dependency in deps.keys():
        if dependency not in service_names:
            raise ConfigurationError(
                "Service '{s.name}' depends on service '{dep}' which is "
                "undefined.".format(s=service_config, dep=dependency)
            )


def get_unsupported_config_msg(path, error_key):
    msg = "Unsupported config option for {}: '{}'".format(path_string(path), error_key)
    if error_key in DOCKER_CONFIG_HINTS:
        msg += " (did you mean '{}'?)".format(DOCKER_CONFIG_HINTS[error_key])
    return msg


def anglicize_json_type(json_type):
    if json_type.startswith(('a', 'e', 'i', 'o', 'u')):
        return 'an ' + json_type
    return 'a ' + json_type


def is_service_dict_schema(schema_id):
    return schema_id in ('config_schema_v1.json', '#/properties/services')


def handle_error_for_schema_with_id(error, path):
    schema_id = error.schema['id']

    if is_service_dict_schema(schema_id) and error.validator == 'additionalProperties':
        return "Invalid service name '{}' - only {} characters are allowed".format(
            # The service_name is one of the keys in the json object
            [i for i in list(error.instance) if not i or any(filter(
                lambda c: not re.match(VALID_NAME_CHARS, c), i
            ))][0],
            VALID_NAME_CHARS
        )

    if error.validator == 'additionalProperties':
        if schema_id == '#/definitions/service':
            invalid_config_key = parse_key_from_error_msg(error)
            return get_unsupported_config_msg(path, invalid_config_key)

        if schema_id.startswith('config_schema_v'):
            invalid_config_key = parse_key_from_error_msg(error)
            return ('Invalid top-level property "{key}". Valid top-level '
                    'sections for this Compose file are: {properties}, and '
                    'extensions starting with "x-".\n\n{explanation}').format(
                key=invalid_config_key,
                properties=', '.join(error.schema['properties'].keys()),
                explanation=VERSION_EXPLANATION
            )

        if not error.path:
            return '{}\n\n{}'.format(error.message, VERSION_EXPLANATION)


def handle_generic_error(error, path):
    msg_format = None
    error_msg = error.message

    if error.validator == 'oneOf':
        msg_format = "{path} {msg}"
        config_key, error_msg = _parse_oneof_validator(error)
        if config_key:
            path.append(config_key)

    elif error.validator == 'type':
        msg_format = "{path} contains an invalid type, it should be {msg}"
        error_msg = _parse_valid_types_from_validator(error.validator_value)

    elif error.validator == 'required':
        error_msg = ", ".join(error.validator_value)
        msg_format = "{path} is invalid, {msg} is required."

    elif error.validator == 'dependencies':
        config_key = list(error.validator_value.keys())[0]
        required_keys = ",".join(error.validator_value[config_key])

        msg_format = "{path} is invalid: {msg}"
        path.append(config_key)
        error_msg = "when defining '{}' you must set '{}' as well".format(
            config_key,
            required_keys)

    elif error.cause:
        error_msg = six.text_type(error.cause)
        msg_format = "{path} is invalid: {msg}"

    elif error.path:
        msg_format = "{path} value {msg}"

    if msg_format:
        return msg_format.format(path=path_string(path), msg=error_msg)

    return error.message


def parse_key_from_error_msg(error):
    return error.message.split("'")[1]


def path_string(path):
    return ".".join(c for c in path if isinstance(c, six.string_types))


def _parse_valid_types_from_validator(validator):
    """A validator value can be either an array of valid types or a string of
    a valid type. Parse the valid types and prefix with the correct article.
    """
    if not isinstance(validator, list):
        return anglicize_json_type(validator)

    if len(validator) == 1:
        return anglicize_json_type(validator[0])

    return "{}, or {}".format(
        ", ".join([anglicize_json_type(validator[0])] + validator[1:-1]),
        anglicize_json_type(validator[-1]))


def _parse_oneof_validator(error):
    """oneOf has multiple schemas, so we need to reason about which schema, sub
    schema or constraint the validation is failing on.
    Inspecting the context value of a ValidationError gives us information about
    which sub schema failed and which kind of error it is.
    """
    types = []
    for context in error.context:
        if context.validator == 'oneOf':
            _, error_msg = _parse_oneof_validator(context)
            return path_string(context.path), error_msg

        if context.validator == 'required':
            return (None, context.message)

        if context.validator == 'additionalProperties':
            invalid_config_key = parse_key_from_error_msg(context)
            return (None, "contains unsupported option: '{}'".format(invalid_config_key))

        if context.validator == 'uniqueItems':
            return (
                path_string(context.path) if context.path else None,
                "contains non-unique items, please remove duplicates from {}".format(
                    context.instance),
            )

        if context.path:
            return (
                path_string(context.path),
                "contains {}, which is an invalid type, it should be {}".format(
                    json.dumps(context.instance),
                    _parse_valid_types_from_validator(context.validator_value)),
            )

        if context.validator == 'type':
            types.append(context.validator_value)

    valid_types = _parse_valid_types_from_validator(types)
    return (None, "contains an invalid type, it should be {}".format(valid_types))


def process_service_constraint_errors(error, service_name, version):
    if version == V1:
        if 'image' in error.instance and 'build' in error.instance:
            return (
                "Service {} has both an image and build path specified. "
                "A service can either be built to image or use an existing "
                "image, not both.".format(service_name))

        if 'image' in error.instance and 'dockerfile' in error.instance:
            return (
                "Service {} has both an image and alternate Dockerfile. "
                "A service can either be built to image or use an existing "
                "image, not both.".format(service_name))

    if 'image' not in error.instance and 'build' not in error.instance:
        return (
            "Service {} has neither an image nor a build context specified. "
            "At least one must be provided.".format(service_name))


def process_config_schema_errors(error):
    path = list(error.path)

    if 'id' in error.schema:
        error_msg = handle_error_for_schema_with_id(error, path)
        if error_msg:
            return error_msg

    return handle_generic_error(error, path)


def validate_against_config_schema(config_file):
    schema = load_jsonschema(config_file)
    format_checker = FormatChecker(["ports", "expose", "subnet_ip_address"])
    validator = Draft4Validator(
        schema,
        resolver=RefResolver(get_resolver_path(), schema),
        format_checker=format_checker)
    handle_errors(
        validator.iter_errors(config_file.config),
        process_config_schema_errors,
        config_file.filename)


def validate_service_constraints(config, service_name, config_file):
    def handler(errors):
        return process_service_constraint_errors(
            errors, service_name, config_file.version)

    schema = load_jsonschema(config_file)
    validator = Draft4Validator(schema['definitions']['constraints']['service'])
    handle_errors(validator.iter_errors(config), handler, None)


def validate_cpu(service_config):
    cpus = service_config.config.get('cpus')
    if not cpus:
        return
    nano_cpus = cpus * NANOCPUS_SCALE
    if isinstance(nano_cpus, float) and not nano_cpus.is_integer():
        raise ConfigurationError(
            "cpus must have nine or less digits after decimal point")


def get_schema_path():
    return os.path.dirname(os.path.abspath(__file__))


def load_jsonschema(config_file):
    filename = os.path.join(
        get_schema_path(),
        "config_schema_v{0}.json".format(config_file.version))

    if not os.path.exists(filename):
        raise ConfigurationError(
            'Version in "{}" is unsupported. {}'
            .format(config_file.filename, VERSION_EXPLANATION))

    with open(filename, "r") as fh:
        return json.load(fh)


def get_resolver_path():
    schema_path = get_schema_path()
    if sys.platform == "win32":
        scheme = "///"
        # TODO: why is this necessary?
        schema_path = schema_path.replace('\\', '/')
    else:
        scheme = "//"
    return "file:{}{}/".format(scheme, schema_path)


def handle_errors(errors, format_error_func, filename):
    """jsonschema returns an error tree full of information to explain what has
    gone wrong. Process each error and pull out relevant information and re-write
    helpful error messages that are relevant.
    """
    errors = list(sorted(errors, key=str))
    if not errors:
        return

    error_msg = '\n'.join(format_error_func(error) for error in errors)
    raise ConfigurationError(
        "The Compose file{file_msg} is invalid because:\n{error_msg}".format(
            file_msg=" '{}'".format(filename) if filename else "",
            error_msg=error_msg))


def validate_healthcheck(service_config):
    healthcheck = service_config.config.get('healthcheck', {})

    if 'test' in healthcheck and isinstance(healthcheck['test'], list):
        if len(healthcheck['test']) == 0:
            raise ConfigurationError(
                'Service "{}" defines an invalid healthcheck: '
                '"test" is an empty list'
                .format(service_config.name))

        # when disable is true config.py::process_healthcheck adds "test: ['NONE']" to service_config
        elif healthcheck['test'][0] == 'NONE' and len(healthcheck) > 1:
            raise ConfigurationError(
                'Service "{}" defines an invalid healthcheck: '
                '"disable: true" cannot be combined with other options'
                .format(service_config.name))

        elif healthcheck['test'][0] not in ('NONE', 'CMD', 'CMD-SHELL'):
            raise ConfigurationError(
                'Service "{}" defines an invalid healthcheck: '
                'when "test" is a list the first item must be either NONE, CMD or CMD-SHELL'
                .format(service_config.name))
