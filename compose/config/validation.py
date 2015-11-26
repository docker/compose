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
VALID_EXPOSE_FORMAT = r'^\d+(\/[a-zA-Z]+)?$'


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
    """
    Check if there is a boolean in the environment and display a warning.
    Always return True here so the validation won't raise an error.
    """
    if isinstance(instance, bool):
        log.warn(
            "There is a boolean value in the 'environment' key.\n"
            "Environment variables can only be strings.\n"
            "Please add quotes to any boolean values to make them string "
            "(eg, 'True', 'yes', 'N').\n"
            "This warning will become an error in a future release. \r\n"
        )
    return True


def validate_top_level_service_objects(config_file):
    """Perform some high level validation of the service name and value.

    This validation must happen before interpolation, which must happen
    before the rest of validation, which is why it's separate from the
    rest of the service validation.
    """
    for service_name, service_dict in config_file.config.items():
        if not isinstance(service_name, six.string_types):
            raise ConfigurationError(
                "In file '{}' service name: {} needs to be a string, eg '{}'".format(
                    config_file.filename,
                    service_name,
                    service_name))

        if not isinstance(service_dict, dict):
            raise ConfigurationError(
                "In file '{}' service '{}' doesn\'t have any configuration options. "
                "All top level keys in your docker-compose.yml must map "
                "to a dictionary of configuration options.".format(
                    config_file.filename,
                    service_name))


def validate_top_level_object(config_file):
    if not isinstance(config_file.config, dict):
        raise ConfigurationError(
            "Top level object in '{}' needs to be an object not '{}'. Check "
            "that you have defined a service at the top level.".format(
                config_file.filename,
                type(config_file.config)))
    validate_top_level_service_objects(config_file)


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


def get_unsupported_config_msg(service_name, error_key):
    msg = "Unsupported config option for '{}' service: '{}'".format(service_name, error_key)
    if error_key in DOCKER_CONFIG_HINTS:
        msg += " (did you mean '{}'?)".format(DOCKER_CONFIG_HINTS[error_key])
    return msg


def anglicize_validator(validator):
    if validator in ["array", "object"]:
        return 'an ' + validator
    return 'a ' + validator


def handle_error_for_schema_with_id(error, service_name):
    schema_id = error.schema['id']

    if schema_id == 'fields_schema.json' and error.validator == 'additionalProperties':
        return "Invalid service name '{}' - only {} characters are allowed".format(
            # The service_name is the key to the json object
            list(error.instance)[0],
            VALID_NAME_CHARS)

    if schema_id == '#/definitions/constraints':
        if 'image' in error.instance and 'build' in error.instance:
            return (
                "Service '{}' has both an image and build path specified. "
                "A service can either be built to image or use an existing "
                "image, not both.".format(service_name))
        if 'image' not in error.instance and 'build' not in error.instance:
            return (
                "Service '{}' has neither an image nor a build path "
                "specified. Exactly one must be provided.".format(service_name))
        if 'image' in error.instance and 'dockerfile' in error.instance:
            return (
                "Service '{}' has both an image and alternate Dockerfile. "
                "A service can either be built to image or use an existing "
                "image, not both.".format(service_name))

    if schema_id == '#/definitions/service':
        if error.validator == 'additionalProperties':
            invalid_config_key = parse_key_from_error_msg(error)
            return get_unsupported_config_msg(service_name, invalid_config_key)


def handle_generic_service_error(error, service_name):
    config_key = " ".join("'%s'" % k for k in error.path)
    msg_format = None
    error_msg = error.message

    if error.validator == 'oneOf':
        msg_format = "Service '{}' configuration key {} {}"
        error_msg = _parse_oneof_validator(error)

    elif error.validator == 'type':
        msg_format = ("Service '{}' configuration key {} contains an invalid "
                      "type, it should be {}")
        error_msg = _parse_valid_types_from_validator(error.validator_value)

    # TODO: no test case for this branch, there are no config options
    # which exercise this branch
    elif error.validator == 'required':
        msg_format = "Service '{}' configuration key '{}' is invalid, {}"

    elif error.validator == 'dependencies':
        msg_format = "Service '{}' configuration key '{}' is invalid: {}"
        config_key = list(error.validator_value.keys())[0]
        required_keys = ",".join(error.validator_value[config_key])
        error_msg = "when defining '{}' you must set '{}' as well".format(
            config_key,
            required_keys)

    elif error.cause:
        error_msg = six.text_type(error.cause)
        msg_format = "Service '{}' configuration key {} is invalid: {}"

    elif error.path:
        msg_format = "Service '{}' configuration key {} value {}"

    if msg_format:
        return msg_format.format(service_name, config_key, error_msg)

    return error.message


def parse_key_from_error_msg(error):
    return error.message.split("'")[1]


def _parse_valid_types_from_validator(validator):
    """A validator value can be either an array of valid types or a string of
    a valid type. Parse the valid types and prefix with the correct article.
    """
    if not isinstance(validator, list):
        return anglicize_validator(validator)

    if len(validator) == 1:
        return anglicize_validator(validator[0])

    return "{}, or {}".format(
        ", ".join([anglicize_validator(validator[0])] + validator[1:-1]),
        anglicize_validator(validator[-1]))


def _parse_oneof_validator(error):
    """oneOf has multiple schemas, so we need to reason about which schema, sub
    schema or constraint the validation is failing on.
    Inspecting the context value of a ValidationError gives us information about
    which sub schema failed and which kind of error it is.
    """
    types = []
    for context in error.context:

        if context.validator == 'required':
            return context.message

        if context.validator == 'additionalProperties':
            invalid_config_key = parse_key_from_error_msg(context)
            return "contains unsupported option: '{}'".format(invalid_config_key)

        if context.path:
            invalid_config_key = " ".join(
                "'{}' ".format(fragment) for fragment in context.path
                if isinstance(fragment, six.string_types)
            )
            return "{}contains {}, which is an invalid type, it should be {}".format(
                invalid_config_key,
                context.instance,
                _parse_valid_types_from_validator(context.validator_value))

        if context.validator == 'uniqueItems':
            return "contains non unique items, please remove duplicates from {}".format(
                context.instance)

        if context.validator == 'type':
            types.append(context.validator_value)

    valid_types = _parse_valid_types_from_validator(types)
    return "contains an invalid type, it should be {}".format(valid_types)


def process_errors(errors, service_name=None):
    """jsonschema gives us an error tree full of information to explain what has
    gone wrong. Process each error and pull out relevant information and re-write
    helpful error messages that are relevant.
    """
    def format_error_message(error, service_name):
        if not service_name and error.path:
            # field_schema errors will have service name on the path
            service_name = error.path.popleft()

        if 'id' in error.schema:
            error_msg = handle_error_for_schema_with_id(error, service_name)
            if error_msg:
                return error_msg

        return handle_generic_service_error(error, service_name)

    return '\n'.join(format_error_message(error, service_name) for error in errors)


def validate_against_fields_schema(config, filename):
    _validate_against_schema(
        config,
        "fields_schema.json",
        format_checker=["ports", "expose", "bool-value-in-mapping"],
        filename=filename)


def validate_against_service_schema(config, service_name):
    _validate_against_schema(
        config,
        "service_schema.json",
        format_checker=["ports"],
        service_name=service_name)


def _validate_against_schema(
        config,
        schema_filename,
        format_checker=(),
        service_name=None,
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

    error_msg = process_errors(errors, service_name)
    file_msg = " in file '{}'".format(filename) if filename else ''
    raise ConfigurationError("Validation failed{}, reason(s):\n{}".format(
        file_msg,
        error_msg))
