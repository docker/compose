import json
import os
from functools import wraps

from docker.utils.ports import split_port
from jsonschema import Draft4Validator
from jsonschema import FormatChecker
from jsonschema import ValidationError

from .errors import ConfigurationError


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


def validate_service_names(func):
    @wraps(func)
    def func_wrapper(config):
        for service_name in config.keys():
            if type(service_name) is int:
                raise ConfigurationError(
                    "Service name: {} needs to be a string, eg '{}'".format(service_name, service_name)
                )
        return func(config)
    return func_wrapper


def validate_top_level_object(func):
    @wraps(func)
    def func_wrapper(config):
        if not isinstance(config, dict):
            raise ConfigurationError(
                "Top level object needs to be a dictionary. Check your .yml file that you have defined a service at the top level."
            )
        return func(config)
    return func_wrapper


def get_unsupported_config_msg(service_name, error_key):
    msg = "Unsupported config option for '{}' service: '{}'".format(service_name, error_key)
    if error_key in DOCKER_CONFIG_HINTS:
        msg += " (did you mean '{}'?)".format(DOCKER_CONFIG_HINTS[error_key])
    return msg


def process_errors(errors):
    """
    jsonschema gives us an error tree full of information to explain what has
    gone wrong. Process each error and pull out relevant information and re-write
    helpful error messages that are relevant.
    """
    def _parse_key_from_error_msg(error):
        return error.message.split("'")[1]

    def _clean_error_message(message):
        return message.replace("u'", "'")

    def _parse_valid_types_from_schema(schema):
        """
        Our defined types using $ref in the schema require some extra parsing
        retrieve a helpful type for error message display.
        """
        if '$ref' in schema:
            return schema['$ref'].replace("#/definitions/", "").replace("_", " ")
        else:
            return str(schema['type'])

    root_msgs = []
    invalid_keys = []
    required = []
    type_errors = []
    other_errors = []

    for error in errors:
        # handle root level errors
        if len(error.path) == 0:
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
            # handle service level errors
            service_name = error.path[0]

            # pop the service name off our path
            error.path.popleft()

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
                else:
                    required.append(_clean_error_message(error.message))
            elif error.validator == 'oneOf':
                config_key = error.path[0]

                valid_types = [_parse_valid_types_from_schema(schema) for schema in error.schema['oneOf']]
                valid_type_msg = " or ".join(valid_types)

                type_errors.append("Service '{}' configuration key '{}' contains an invalid type, valid types are {}".format(
                    service_name, config_key, valid_type_msg)
                )
            elif error.validator == 'type':
                msg = "a"
                if error.validator_value == "array":
                    msg = "an"

                if len(error.path) > 0:
                    config_key = " ".join(["'%s'" % k for k in error.path])
                    type_errors.append(
                        "Service '{}' configuration key {} contains an invalid "
                        "type, it should be {} {}".format(
                            service_name,
                            config_key,
                            msg,
                            error.validator_value))
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


def validate_against_schema(config):
    config_source_dir = os.path.dirname(os.path.abspath(__file__))
    schema_file = os.path.join(config_source_dir, "schema.json")

    with open(schema_file, "r") as schema_fh:
        schema = json.load(schema_fh)

    validation_output = Draft4Validator(schema, format_checker=FormatChecker(["ports"]))

    errors = [error for error in sorted(validation_output.iter_errors(config), key=str)]
    if errors:
        error_msg = process_errors(errors)
        raise ConfigurationError("Validation failed, reason(s):\n{}".format(error_msg))
