import os
import yaml
import six

from .const import ALLOWED_KEYS
from .errors import ConfigurationError, CircularReference, ValidationError
from .validators import EnvironmentValidator, FileValidator, ServiceValidator, ERROR_BAD_TYPE, ERROR_UNACCESSIBLE_PATH


def load(filename):
    working_dir = os.path.dirname(filename)
    file_validator = FileValidator(filename=filename)
    services_dict = load_yaml(filename)
    if not file_validator.validate({'file': services_dict}):
        raise ValidationError(filename, file_validator.errors)
    return from_dictionary(services_dict, working_dir=working_dir, filename=filename)


def from_dictionary(dictionary, working_dir=None, filename=None):
    if not isinstance(dictionary, dict):
        raise RuntimeError('A dict must be passed as `dictionary` to `config.from_dictionary`')

    service_dicts = []
    errors = dict()

    for service_name, service_dict in list(dictionary.items()):
        if not isinstance(service_dict, dict):
            raise ConfigurationError('Service "%s" doesn\'t have any configuration options. All top level keys in your docker-compose.yml must map to a dictionary of configuration options.' % service_name)
        loader = ServiceLoader(name=service_name, working_dir=working_dir, filename=filename)
        service_dict, _errors = loader.make_service_dict(service_name, service_dict)
        service_dicts.append(service_dict)
        errors.update(_errors)

    if errors:
        raise ValidationError(filename, errors)

    return service_dicts


# TODO? as this is only used in tests, it should be moved there
# FIXME? pass on ValidationError in order to let tests pass unparsable objects in testcases.DockerClientTestCase.create_service
def make_service_dict(name, service_dict, working_dir=None):
    service_dict, errors = ServiceLoader(working_dir=working_dir).make_service_dict(name, service_dict)
    if not errors:
        return service_dict
    else:
        raise ValidationError(name, errors)


class ServiceLoader(object):
    def __init__(self, working_dir, name=None, filename=None, already_seen=None, validator=None):
        # name will be used for prefixing error messages
        self.working_dir = working_dir
        self.filename = filename
        self.already_seen = already_seen or []
        self.validator = validator or ServiceValidator(service_name=name, working_dir=self.working_dir)

    def make_service_dict(self, name, service_dict, is_extended=False):
        if self.signature(name) in self.already_seen:
            raise CircularReference(self.already_seen)

        service_dict = service_dict.copy()
        service_dict['name'] = name

        service_dict = self.process_container_options(service_dict)

        if not is_extended:
            _errors = self.validator.errors
            self.validator.validate(service_dict)
            self.validator._errors.update(_errors)

        return service_dict, self.validator.errors

    def process_container_options(self, service_dict):
        service_dict = self.resolve_environment(service_dict)
        service_dict = self.resolve_extends(service_dict)

        service_dict = service_dict.copy()

        if 'volumes' in service_dict:
            service_dict['volumes'] = self.resolve_host_paths(service_dict['volumes'])

        if 'build' in service_dict:
            if not isinstance(service_dict['build'], six.string_types):
                self.validator._error('build', ERROR_BAD_TYPE % 'string')
            service_dict['build'] = self.resolve_build_path(service_dict['build'])

        return service_dict

    def resolve_environment(self, service_dict):
        service_dict = service_dict.copy()

        if 'environment' not in service_dict and 'env_file' not in service_dict:
            return service_dict

        env = {}

        if 'env_file' in service_dict:
            for f in self.get_env_files(service_dict['env_file']):
                env.update(self.env_vars_from_file(f))
            del service_dict['env_file']

        env.update(parse_environment(service_dict.get('environment'), self.validator))
        env = dict(resolve_env_var(k, v) for k, v in six.iteritems(env))

        service_dict['environment'] = env
        return service_dict

    def get_env_files(self, env_files):
        if isinstance(env_files, six.string_types):
            env_files = [env_files]
        if not isinstance(env_files, list):
            self.validator._error('env_file', ERROR_BAD_TYPE % 'string or list of strings')

        return [expand_path(self.working_dir, path) for path in env_files]

    def env_vars_from_file(self, filename):
        """
        Read in a line delimited file of environment variables.
        """
        env = {}
        if not (os.path.isfile(filename) and os.access(filename, os.R_OK)):
            self.validator._error('env_file', ERROR_UNACCESSIBLE_PATH % filename)
            return env
        for line in open(filename, 'r'):
            line = line.strip()
            if line and not line.startswith('#'):
                k, v = split_env(line)
                env[k] = v
        return env

    def resolve_extends(self, service_dict):
        if 'extends' not in service_dict:
            return service_dict

        extends_options = self.validate_extends_options(service_dict['extends'])
        if extends_options is None:
            return service_dict

        other_config_path = expand_path(self.working_dir, extends_options['file'])
        other_working_dir = os.path.dirname(other_config_path)
        other_already_seen = self.already_seen + [self.signature(service_dict['name'])]
        other_loader = ServiceLoader(working_dir=other_working_dir, filename=other_config_path,
                                     name=service_dict['name'] + ' extends ' + other_config_path + '#',
                                     already_seen=other_already_seen, validator=self.validator)

        try:
            other_config = load_yaml(other_config_path)
        except ConfigurationError, e:
            self.validator._error('extends', e.msg)

        try:
            other_service_dict = other_config[extends_options['service']]
        except KeyError, e:  # noqa
            self.validator._error(' extends ' + other_config_path,
                                  '%s is not defined' % extends_options['service'])

        other_service_dict, errors = other_loader.make_service_dict(
            service_dict['name'],
            other_service_dict,
            is_extended=True
        )

        self.validate_extended_service_dict(service_dict=other_service_dict, extends_filename=other_config_path,
                                            extended_service=extends_options['service'])

        return merge_service_dicts(other_service_dict, service_dict, self.validator)

    def resolve_host_paths(self, volumes):

        def resolve_host_path(volume, working_dir):
            container_path, host_path = split_path_mapping(volume, self.validator)
            if host_path is not None:
                host_path = os.path.expanduser(host_path)
                host_path = os.path.expandvars(host_path)
                return "%s:%s" % (expand_path(working_dir, host_path), container_path)
            else:
                return container_path

        return [resolve_host_path(v, self.working_dir) for v in volumes]

    def resolve_build_path(self, build_path):
        if not isinstance(build_path, six.string_types):
            return build_path
        return expand_path(self.working_dir, build_path)

    def signature(self, name):
        return self.filename, name

    def validate_extends_options(self, extends_options):
        if not isinstance(extends_options, dict):
            self.validator._error('extends', ERROR_BAD_TYPE % 'dict')
            return None

        if set(extends_options.keys()) != set(['file', 'service']):
            self.validator._error('extends', 'only `file` and `service` must be given')
            return None

        return extends_options

    def validate_extended_service_dict(self, service_dict, extends_filename, extended_service):
        source = ' extends ' + extends_filename

        if 'links' in service_dict:
            self.validator._error(source, {extended_service: 'services with `links` cannot be extended'})

        if 'volumes_from' in service_dict:
            self.validator._error(source, {extended_service: 'services with `volumes_from` cannot be extended'})

        if 'net' in service_dict:
            if get_service_name_from_net(service_dict['net']) is not None:
                self.validator._error(source, {extended_service: 'services with `net: container` cannot be extended'})


def merge_service_dicts(base, override, validator=None):
    d = base.copy()

    if 'environment' in base or 'environment' in override:
        d['environment'] = merge_environment(
            base.get('environment'),
            override.get('environment'),
            validator
        )

    path_mapping_keys = ['volumes', 'devices']

    for key in path_mapping_keys:
        if key in base or key in override:
            d[key] = merge_path_mappings(
                base.get(key),
                override.get(key),
            )

    if 'labels' in base or 'labels' in override:
        d['labels'] = merge_labels(
            base.get('labels'),
            override.get('labels'),
        )

    if 'image' in override and 'build' in d:
        del d['build']

    if 'build' in override and 'image' in d:
        del d['image']

    list_keys = ['ports', 'expose', 'external_links']

    for key in list_keys:
        if key in base or key in override:
            d[key] = base.get(key, []) + override.get(key, [])

    list_or_string_keys = ['dns', 'dns_search']

    for key in list_or_string_keys:
        if key in base or key in override:
            d[key] = to_list(base.get(key)) + to_list(override.get(key))

    already_merged_keys = ['environment', 'labels'] + path_mapping_keys + list_keys + list_or_string_keys

    for k in ALLOWED_KEYS - set(already_merged_keys):
        if k in override:
            d[k] = override[k]

    return d


def merge_environment(base, override, validator=None):
    env = parse_environment(base, validator)
    env.update(parse_environment(override, validator))
    return env


def parse_links(links):
    mappings = []
    for link in links:
        if ':' in link:
            mappings.append(link.split(':', 1))
        else:
            mappings.append((link, link))
    return dict(mappings)


def split_env(env):
    if '=' in env:
        return env.split('=', 1)
    else:
        return env, None


def parse_environment(environment, validator=None):
    if not environment:
        return {}

    if isinstance(environment, list):
        environment = dict(split_env(e) for e in environment)

    if validator:
        env_validator = EnvironmentValidator()
        if not env_validator.validate({'environment': environment}, update=True):
            validator._errors.update(env_validator.errors)
            return {}
    elif not isinstance(environment, dict):
        raise ConfigurationError("environment \"%s\" must be a list or mapping," % environment)

    return environment


def resolve_env_var(key, val):
    if val is not None:
        return key, val
    elif key in os.environ:
        return key, os.environ[key]
    else:
        return key, ''


def merge_volumes(base, override):
    d = dict_from_path_mappings(base)
    d.update(dict_from_path_mappings(override))
    return path_mappings_from_dict(d)


def merge_path_mappings(base, override):
    d = dict_from_path_mappings(base)
    d.update(dict_from_path_mappings(override))
    return path_mappings_from_dict(d)


def dict_from_path_mappings(path_mappings):
    if path_mappings:
        return dict(split_path_mapping(v) for v in path_mappings)
    else:
        return {}


def path_mappings_from_dict(d):
    return [join_path_mapping(v) for v in d.items()]


def split_path_mapping(string, validator=None):
    if ':' in string:
        (host, container) = string.split(':', 1)
        if validator and not (host and container):
            validator._error('volume', 'host- and container-path must be empty')
        return container, host
    else:
        return string, None


def join_path_mapping(pair):
    (container, host) = pair
    if host is None:
        return container
    else:
        return ":".join((host, container))


def merge_labels(base, override):
    labels = parse_labels(base)
    labels.update(parse_labels(override))
    return labels


def parse_labels(labels):
    if not labels:
        return {}

    if isinstance(labels, list):
        return dict(split_label(e) for e in labels)

    if isinstance(labels, dict):
        return labels

    raise ConfigurationError(
        "labels \"%s\" must be a list or mapping" %
        labels
    )


def split_label(label):
    if '=' in label:
        return label.split('=', 1)
    else:
        return label, ''


def expand_path(working_dir, path):
    return os.path.abspath(os.path.join(working_dir, path))


def to_list(value):
    if value is None:
        return []
    elif isinstance(value, six.string_types):
        return [value]
    else:
        return value


def get_service_name_from_net(net_config):
    if not net_config:
        return

    if not net_config.startswith('container:'):
        return

    return net_config.split(':', 1)[1]


def load_yaml(filename):
    try:
        with open(filename, 'r') as fh:
            return yaml.safe_load(fh)
    except (IOError, yaml.YAMLError) as e:
        raise ConfigurationError(six.text_type(e))
