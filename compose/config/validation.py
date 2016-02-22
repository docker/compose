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


@FormatChecker.cls_checks(format="bool-value-in-mapping")
def format_boolean_in_environment(instance):
    """Check if there is a boolean in the mapping sections and display a warning.
    Always return True here so the validation won't raise an error.
    """
    if isinstance(instance, bool):
        log.warn(
            "There is a boolean value in the 'environment', 'labels', or "
            "'extra_hosts' field of a service.\n"
            "These sections only support string values.\n"
            "Please add quotes to any boolean values to make them strings "
            "(eg, 'True', 'false', 'yes', 'N', 'on', 'Off').\n"
            "This warning will become an error in a future release. \r\n"
        )
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


def validate_depends_on(service_config, service_names):
    for dependency in service_config.config.get('depends_on', []):
        if dependency not in service_names:
            raise ConfigurationError(
                "Service '{s.name}' depends on service '{dep}' which is "
                "undefined.".format(s=service_config, dep=dependency))


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
    return schema_id == 'fields_schema_v1.json' or schema_id == '#/properties/services'


def handle_error_for_schema_with_id(error, path):
    schema_id = error.schema['id']

    if is_service_dict_schema(schema_id) and error.validator == 'additionalProperties':
        return "Invalid service name '{}' - only {} characters are allowed".format(
            # The service_name is the key to the json object
            list(error.instance)[0],
            VALID_NAME_CHARS)

    if schema_id == '#/definitions/constraints':
        # Build context could in 'build' or 'build.context' and dockerfile could be
        # in 'dockerfile' or 'build.dockerfile'
        context = False
        dockerfile = 'dockerfile' in error.instance
        if 'build' in error.instance:
            if isinstance(error.instance['build'], six.string_types):
                context = True
            else:
                context = 'context' in error.instance['build']
                dockerfile = dockerfile or 'dockerfile' in error.instance['build']

        # TODO: only applies to v1
        if 'image' in error.instance and context:
            return (
                "{} has both an image and build path specified. "
                "A service can either be built to image or use an existing "
                "image, not both.".format(path_string(path)))
        if 'image' not in error.instance and not context:
            return (
                "{} has neither an image nor a build path specified. "
                "At least one must be provided.".format(path_string(path)))
        # TODO: only applies to v1
        if 'image' in error.instance and dockerfile:
            return (
                "{} has both an image and alternate Dockerfile. "
                "A service can either be built to image or use an existing "
                "image, not both.".format(path_string(path)))

    if error.validator == 'additionalProperties':
        if schema_id == '#/definitions/service':
            invalid_config_key = parse_key_from_error_msg(error)
            return get_unsupported_config_msg(path, invalid_config_key)

        if not error.path:
            return '{}\n{}'.format(error.message, VERSION_EXPLANATION)


def handle_generic_service_error(error, path):
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

        if context.path:
            return (
                path_string(context.path),
                "contains {}, which is an invalid type, it should be {}".format(
                    json.dumps(context.instance),
                    _parse_valid_types_from_validator(context.validator_value)),
            )

        if context.validator == 'uniqueItems':
            return (
                None,
                "contains non unique items, please remove duplicates from {}".format(
                    context.instance),
            )

        if context.validator == 'type':
            types.append(context.validator_value)

    valid_types = _parse_valid_types_from_validator(types)
    return (None, "contains an invalid type, it should be {}".format(valid_types))


def process_errors(errors, path_prefix=None):
    """jsonschema gives us an error tree full of information to explain what has
    gone wrong. Process each error and pull out relevant information and re-write
    helpful error messages that are relevant.
    """
    path_prefix = path_prefix or []

    def format_error_message(error):
        path = path_prefix + list(error.path)

        if 'id' in error.schema:
            error_msg = handle_error_for_schema_with_id(error, path)
            if error_msg:
                return error_msg

        return handle_generic_service_error(error, path)

    return '\n'.join(format_error_message(error) for error in errors)


def validate_against_fields_schema(config_file):
    schema_filename = "fields_schema_v{0}.json".format(config_file.version)
    _validate_against_schema(
        config_file.config,
        schema_filename,
        format_checker=["ports", "expose", "bool-value-in-mapping"],
        filename=config_file.filename)


def validate_against_service_schema(config, service_name, version):
    _validate_against_schema(
        config,
        "service_schema_v{0}.json".format(version),
        format_checker=["ports"],
        path_prefix=[service_name])


def _validate_against_schema(
        config,
        schema_filename,
        format_checker=(),
        path_prefix=None,
        filename=None):
    config_source_dir = os.path.dirname(os.path.abspath(__file__))

    if sys.platform == "win32":
        file_pre_fix = "///"
        config_source_dir = config_source_dir.replace('\\', '/')
    else:
        file_pre_fix = "//"

    resolver_full_path = "file:{}{}/".format(file_pre_fix, config_source_dir)
    schema_file = os.path.join(config_source_dir, schema_filename)

    with open(schema_file, "r") as schema_fh:
        schema = json.load(schema_fh)

    resolver = RefResolver(resolver_full_path, schema)
    validation_output = Draft4Validator(
        schema,
        resolver=resolver,
        format_checker=FormatChecker(format_checker))

    errors = [error for error in sorted(validation_output.iter_errors(config), key=str)]
    if not errors:
        return

    error_msg = process_errors(errors, path_prefix=path_prefix)
    file_msg = " in file '{}'".format(filename) if filename else ''
    raise ConfigurationError("Validation failed{}, reason(s):\n{}".format(
        file_msg,
        error_msg))
