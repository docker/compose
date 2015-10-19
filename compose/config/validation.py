import json
import logging
import os
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


@FormatChecker.cls_checks(
    format="ports",
    raises=ValidationError(
        "Invalid port formatting, it should be "
        "'[[remote_ip:]remote_port:]port[/protocol]'"))
def format_ports(instance):
    try:
        split_port(instance)
    except ValueError:
        return False
    return True


@FormatChecker.cls_checks(format="environment")
def format_boolean_in_environment(instance):
    """
    Check if there is a boolean in the environment and display a warning.
    Always return True here so the validation won't raise an error.
    """
    if isinstance(instance, bool):
        log.warn(
            "Warning: There is a boolean value in the 'environment' key.\n"
            "Environment variables can only be strings.\nPlease add quotes to any boolean values to make them string "
            "(eg, 'True', 'yes', 'N').\nThis warning will become an error in a future release. \r\n"
        )
    return True


def validate_service_names(config):
    for service_name in config.keys():
        if not isinstance(service_name, six.string_types):
            raise ConfigurationError(
                "Service name: {} needs to be a string, eg '{}'".format(
                    service_name,
                    service_name))


def validate_top_level_object(config):
    if not isinstance(config, dict):
        raise ConfigurationError(
            "Top level object needs to be a dictionary. Check your .yml file "
            "that you have defined a service at the top level.")
    validate_service_names(config)


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


def validate_extended_service_exists(extended_service_name, full_extended_config, extended_config_path):
    if extended_service_name not in full_extended_config:
        msg = (
            "Cannot extend service '%s' in %s: Service not found"
        ) % (extended_service_name, extended_config_path)
        raise ConfigurationError(msg)


def get_unsupported_config_msg(service_name, error_key):
    msg = "Unsupported config option for '{}' service: '{}'".format(service_name, error_key)
    if error_key in DOCKER_CONFIG_HINTS:
        msg += " (did you mean '{}'?)".format(DOCKER_CONFIG_HINTS[error_key])
    return msg


def anglicize_validator(validator):
    if validator in ["array", "object"]:
        return 'an ' + validator
    return 'a ' + validator


def process_errors(errors, service_name=None):
    """
    jsonschema gives us an error tree full of information to explain what has
    gone wrong. Process each error and pull out relevant information and re-write
    helpful error messages that are relevant.
    """
    def _parse_key_from_error_msg(error):
        return error.message.split("'")[1]

    def _clean_error_message(message):
        return message.replace("u'", "'")

    def _parse_valid_types_from_validator(validator):
        """
        A validator value can be either an array of valid types or a string of
        a valid type. Parse the valid types and prefix with the correct article.
        """
        if isinstance(validator, list):
            if len(validator) >= 2:
                first_type = anglicize_validator(validator[0])
                last_type = anglicize_validator(validator[-1])
                types_from_validator = "{}{}".format(first_type, ", ".join(validator[1:-1]))

                msg = "{} or {}".format(
                    types_from_validator,
                    last_type
                )
            else:
                msg = "{}".format(anglicize_validator(validator[0]))
        else:
            msg = "{}".format(anglicize_validator(validator))

        return msg

    def _parse_oneof_validator(error):
        """
        oneOf has multiple schemas, so we need to reason about which schema, sub
        schema or constraint the validation is failing on.
        Inspecting the context value of a ValidationError gives us information about
        which sub schema failed and which kind of error it is.
        """

        required = [context for context in error.context if context.validator == 'required']
        if required:
            return required[0].message

        additionalProperties = [context for context in error.context if context.validator == 'additionalProperties']
        if additionalProperties:
            invalid_config_key = _parse_key_from_error_msg(additionalProperties[0])
            return "contains unsupported option: '{}'".format(invalid_config_key)

        constraint = [context for context in error.context if len(context.path) > 0]
        if constraint:
            valid_types = _parse_valid_types_from_validator(constraint[0].validator_value)
            invalid_config_key = "".join(
                "'{}' ".format(fragment) for fragment in constraint[0].path
                if isinstance(fragment, six.string_types)
            )
            msg = "{}contains {}, which is an invalid type, it should be {}".format(
                invalid_config_key,
                constraint[0].instance,
                valid_types
            )
            return msg

        uniqueness = [context for context in error.context if context.validator == 'uniqueItems']
        if uniqueness:
            msg = "contains non unique items, please remove duplicates from {}".format(
                uniqueness[0].instance
            )
            return msg

        types = [context.validator_value for context in error.context if context.validator == 'type']
        valid_types = _parse_valid_types_from_validator(types)

        msg = "contains an invalid type, it should be {}".format(valid_types)

        return msg

    root_msgs = []
    invalid_keys = []
    required = []
    type_errors = []
    other_errors = []

    for error in errors:
        # handle root level errors
        if len(error.path) == 0 and not error.instance.get('name'):
            if error.validator == 'type':
                msg = "Top level object needs to be a dictionary. Check your .yml file that you have defined a service at the top level."
                root_msgs.append(msg)
            elif error.validator == 'additionalProperties':
                invalid_service_name = _parse_key_from_error_msg(error)
                msg = "Invalid service name '{}' - only {} characters are allowed".format(invalid_service_name, VALID_NAME_CHARS)
                root_msgs.append(msg)
            else:
                root_msgs.append(_clean_error_message(error.message))

        else:
            if not service_name:
                # field_schema errors will have service name on the path
                service_name = error.path[0]
                error.path.popleft()
            else:
                # service_schema errors have the service name passed in, as that
                # is not available on error.path or necessarily error.instance
                service_name = service_name

            if error.validator == 'additionalProperties':
                invalid_config_key = _parse_key_from_error_msg(error)
                invalid_keys.append(get_unsupported_config_msg(service_name, invalid_config_key))
            elif error.validator == 'anyOf':
                if 'image' in error.instance and 'build' in error.instance:
                    required.append(
                        "Service '{}' has both an image and build path specified. "
                        "A service can either be built to image or use an existing "
                        "image, not both.".format(service_name))
                elif 'image' not in error.instance and 'build' not in error.instance:
                    required.append(
                        "Service '{}' has neither an image nor a build path "
                        "specified. Exactly one must be provided.".format(service_name))
                elif 'image' in error.instance and 'dockerfile' in error.instance:
                    required.append(
                        "Service '{}' has both an image and alternate Dockerfile. "
                        "A service can either be built to image or use an existing "
                        "image, not both.".format(service_name))
                else:
                    required.append(_clean_error_message(error.message))
            elif error.validator == 'oneOf':
                config_key = error.path[0]
                msg = _parse_oneof_validator(error)

                type_errors.append("Service '{}' configuration key '{}' {}".format(
                    service_name, config_key, msg)
                )
            elif error.validator == 'type':
                msg = _parse_valid_types_from_validator(error.validator_value)

                if len(error.path) > 0:
                    config_key = " ".join(["'%s'" % k for k in error.path])
                    type_errors.append(
                        "Service '{}' configuration key {} contains an invalid "
                        "type, it should be {}".format(
                            service_name,
                            config_key,
                            msg))
                else:
                    root_msgs.append(
                        "Service '{}' doesn\'t have any configuration options. "
                        "All top level keys in your docker-compose.yml must map "
                        "to a dictionary of configuration options.'".format(service_name))
            elif error.validator == 'required':
                config_key = error.path[0]
                required.append(
                    "Service '{}' option '{}' is invalid, {}".format(
                        service_name,
                        config_key,
                        _clean_error_message(error.message)))
            elif error.validator == 'dependencies':
                dependency_key = list(error.validator_value.keys())[0]
                required_keys = ",".join(error.validator_value[dependency_key])
                required.append("Invalid '{}' configuration for '{}' service: when defining '{}' you must set '{}' as well".format(
                    dependency_key, service_name, dependency_key, required_keys))
            else:
                config_key = " ".join(["'%s'" % k for k in error.path])
                err_msg = "Service '{}' configuration key {} value {}".format(service_name, config_key, error.message)
                other_errors.append(err_msg)

    return "\n".join(root_msgs + invalid_keys + required + type_errors + other_errors)


def validate_against_fields_schema(config):
    schema_filename = "fields_schema.json"
    format_checkers = ["ports", "environment"]
    return _validate_against_schema(config, schema_filename, format_checkers)


def validate_against_service_schema(config, service_name):
    schema_filename = "service_schema.json"
    format_checkers = ["ports"]
    return _validate_against_schema(config, schema_filename, format_checkers, service_name)


def _validate_against_schema(config, schema_filename, format_checker=[], service_name=None):
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
    validation_output = Draft4Validator(schema, resolver=resolver, format_checker=FormatChecker(format_checker))

    errors = [error for error in sorted(validation_output.iter_errors(config), key=str)]
    if errors:
        error_msg = process_errors(errors, service_name)
        raise ConfigurationError("Validation failed, reason(s):\n{}".format(error_msg))
