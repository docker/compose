import os
import yaml
import six


DOCKER_CONFIG_KEYS = [
    'cap_add',
    'cap_drop',
    'cpu_shares',
    'command',
    'detach',
    'dns',
    'dns_search',
    'domainname',
    'entrypoint',
    'env_file',
    'environment',
    'hostname',
    'image',
    'links',
    'mem_limit',
    'net',
    'ports',
    'privileged',
    'restart',
    'stdin_open',
    'tty',
    'user',
    'volumes',
    'volumes_from',
    'working_dir',
]

ALLOWED_KEYS = DOCKER_CONFIG_KEYS + [
    'build',
    'expose',
    'external_links',
    'name',
]

DOCKER_CONFIG_HINTS = {
    'cpu_share' : 'cpu_shares',
    'link'      : 'links',
    'port'      : 'ports',
    'privilege' : 'privileged',
    'priviliged': 'privileged',
    'privilige' : 'privileged',
    'volume'    : 'volumes',
    'workdir'   : 'working_dir',
}


def load(filename):
    return from_dictionary(load_yaml(filename))


def load_yaml(filename):
    try:
        with open(filename, 'r') as fh:
            return yaml.safe_load(fh)
    except IOError as e:
        raise ConfigurationError(six.text_type(e))


def from_dictionary(dictionary):
    service_dicts = []

    for service_name, service_dict in list(dictionary.items()):
        if not isinstance(service_dict, dict):
            raise ConfigurationError('Service "%s" doesn\'t have any configuration options. All top level keys in your docker-compose.yml must map to a dictionary of configuration options.' % service_name)
        service_dict = make_service_dict(service_name, service_dict)
        service_dicts.append(service_dict)

    return service_dicts


def make_service_dict(name, options):
    service_dict = options.copy()
    service_dict['name'] = name
    return process_container_options(service_dict)


def process_container_options(service_dict):
    for k in service_dict:
        if k not in ALLOWED_KEYS:
            msg = "Unsupported config option for %s service: '%s'" % (service_dict['name'], k)
            if k in DOCKER_CONFIG_HINTS:
                msg += " (did you mean '%s'?)" % DOCKER_CONFIG_HINTS[k]
            raise ConfigurationError(msg)

    for filename in get_env_files(service_dict):
        if not os.path.exists(filename):
            raise ConfigurationError("Couldn't find env file for service %s: %s" % (service_dict['name'], filename))

    if 'environment' in service_dict or 'env_file' in service_dict:
        service_dict['environment'] = build_environment(service_dict)

    return service_dict


def merge_service_dicts(base, override):
    d = base.copy()

    if 'environment' in base or 'environment' in override:
        d['environment'] = merge_environment(base, override)

    if 'links' in base or 'links' in override:
        d['links'] = merge_links(base, override)

    for k in ALLOWED_KEYS:
        if k not in ['links', 'environment']:
            if k in override:
                d[k] = override[k]

    return d


def merge_environment(base, override):
    env = build_environment(base)
    env.update(build_environment(override))
    return env


def merge_links(base, override):
    d = parse_links(base.get('links', []))
    d.update(parse_links(override.get('links', [])))
    return ["%s:%s" % (source, alias) for (alias, source) in six.iteritems(d)]


def parse_links(links):
    return dict(parse_link(l) for l in links)


def parse_link(link):
    if ':' in link:
        source, alias = link.split(':', 1)
        return (alias, source)
    else:
        return (link, link)


def get_env_files(options):
    env_files = options.get('env_file', [])
    if not isinstance(env_files, list):
        env_files = [env_files]
    return env_files


def build_environment(options):
    env = {}

    for f in get_env_files(options):
        env.update(env_vars_from_file(f))

    env.update(parse_environment(options.get('environment')))
    return dict(resolve_env(k, v) for k, v in six.iteritems(env))


def parse_environment(environment):
    if not environment:
        return {}

    if isinstance(environment, list):
        return dict(split_env(e) for e in environment)

    if isinstance(environment, dict):
        return environment

    raise ConfigurationError(
        "environment \"%s\" must be a list or mapping," %
        environment
    )


def split_env(env):
    if '=' in env:
        return env.split('=', 1)
    else:
        return env, None


def resolve_env(key, val):
    if val is not None:
        return key, val
    elif key in os.environ:
        return key, os.environ[key]
    else:
        return key, ''


def env_vars_from_file(filename):
    """
    Read in a line delimited file of environment variables.
    """
    env = {}
    for line in open(filename, 'r'):
        line = line.strip()
        if line and not line.startswith('#'):
            k, v = split_env(line)
            env[k] = v
    return env


class ConfigurationError(Exception):
    def __init__(self, msg):
        self.msg = msg

    def __str__(self):
        return self.msg
