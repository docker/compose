# Copyright 2013 dotCloud inc.

#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at

#        http://www.apache.org/licenses/LICENSE-2.0

#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.

import base64
import fileinput
import json
import os

import six

from ..utils import utils

INDEX_URL = 'https://index.docker.io/v1/'
DOCKER_CONFIG_FILENAME = '.dockercfg'


def swap_protocol(url):
    if url.startswith('http://'):
        return url.replace('http://', 'https://', 1)
    if url.startswith('https://'):
        return url.replace('https://', 'http://', 1)
    return url


def expand_registry_url(hostname):
    if hostname.startswith('http:') or hostname.startswith('https:'):
        if '/' not in hostname[9:]:
            hostname = hostname + '/v1/'
        return hostname
    if utils.ping('https://' + hostname + '/v1/_ping'):
        return 'https://' + hostname + '/v1/'
    return 'http://' + hostname + '/v1/'


def resolve_repository_name(repo_name):
    if '://' in repo_name:
        raise ValueError('Repository name cannot contain a '
                         'scheme ({0})'.format(repo_name))
    parts = repo_name.split('/', 1)
    if not '.' in parts[0] and not ':' in parts[0] and parts[0] != 'localhost':
        # This is a docker index repo (ex: foo/bar or ubuntu)
        return INDEX_URL, repo_name
    if len(parts) < 2:
        raise ValueError('Invalid repository name ({0})'.format(repo_name))

    if 'index.docker.io' in parts[0]:
        raise ValueError('Invalid repository name,'
                         'try "{0}" instead'.format(parts[1]))

    return expand_registry_url(parts[0]), parts[1]


def resolve_authconfig(authconfig, registry=None):
    """Return the authentication data from the given auth configuration for a
    specific registry. We'll do our best to infer the correct URL for the
    registry, trying both http and https schemes. Returns an empty dictionnary
    if no data exists."""
    # Default to the public index server
    registry = registry or INDEX_URL

    # Ff its not the index server there are three cases:
    #
    # 1. this is a full config url -> it should be used as is
    # 2. it could be a full url, but with the wrong protocol
    # 3. it can be the hostname optionally with a port
    #
    # as there is only one auth entry which is fully qualified we need to start
    # parsing and matching
    if '/' not in registry:
        registry = registry + '/v1/'
    if not registry.startswith('http:') and not registry.startswith('https:'):
        registry = 'https://' + registry

    if registry in authconfig:
        return authconfig[registry]
    return authconfig.get(swap_protocol(registry), None)


def decode_auth(auth):
    if isinstance(auth, six.string_types):
        auth = auth.encode('ascii')
    s = base64.b64decode(auth)
    login, pwd = s.split(b':')
    return login.decode('ascii'), pwd.decode('ascii')


def encode_header(auth):
    auth_json = json.dumps(auth).encode('ascii')
    return base64.b64encode(auth_json)


def load_config(root=None):
    """Loads authentication data from a Docker configuration file in the given
    root directory."""
    conf = {}
    data = None

    config_file = os.path.join(root or os.environ.get('HOME', '.'),
                               DOCKER_CONFIG_FILENAME)

    # First try as JSON
    try:
        with open(config_file) as f:
            conf = {}
            for registry, entry in six.iteritems(json.load(f)):
                username, password = decode_auth(entry['auth'])
                conf[registry] = {
                    'username': username,
                    'password': password,
                    'email': entry['email'],
                    'serveraddress': registry,
                }
            return conf
    except:
        pass

    # If that fails, we assume the configuration file contains a single
    # authentication token for the public registry in the following format:
    #
    # auth = AUTH_TOKEN
    # email = email@domain.com
    try:
        data = []
        for line in fileinput.input(config_file):
            data.append(line.strip().split(' = ')[1])
        if len(data) < 2:
            # Not enough data
            raise Exception('Invalid or empty configuration file!')

        username, password = decode_auth(data[0])
        conf[INDEX_URL] = {
            'username': username,
            'password': password,
            'email': data[1],
            'serveraddress': INDEX_URL,
        }
        return conf
    except:
        pass

    # If all fails, return an empty config
    return {}
