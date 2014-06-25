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

import json
import re
import shlex
import struct
import warnings

import requests
import requests.exceptions
from fig.packages import six

from .auth import auth
from .unixconn import unixconn
from .utils import utils
from . import errors

if not six.PY3:
    import websocket

DEFAULT_DOCKER_API_VERSION = '1.12'
DEFAULT_TIMEOUT_SECONDS = 60
STREAM_HEADER_SIZE_BYTES = 8


class Client(requests.Session):
    def __init__(self, base_url=None, version=DEFAULT_DOCKER_API_VERSION,
                 timeout=DEFAULT_TIMEOUT_SECONDS):
        super(Client, self).__init__()
        if base_url is None:
            base_url = "http+unix://var/run/docker.sock"
        if 'unix:///' in base_url:
            base_url = base_url.replace('unix:/', 'unix:')
        if base_url.startswith('unix:'):
            base_url = "http+" + base_url
        if base_url.startswith('tcp:'):
            base_url = base_url.replace('tcp:', 'http:')
        if base_url.endswith('/'):
            base_url = base_url[:-1]
        self.base_url = base_url
        self._version = version
        self._timeout = timeout
        self._auth_configs = auth.load_config()

        self.mount('http+unix://', unixconn.UnixAdapter(base_url, timeout))

    def _set_request_timeout(self, kwargs):
        """Prepare the kwargs for an HTTP request by inserting the timeout
        parameter, if not already present."""
        kwargs.setdefault('timeout', self._timeout)
        return kwargs

    def _post(self, url, **kwargs):
        return self.post(url, **self._set_request_timeout(kwargs))

    def _get(self, url, **kwargs):
        return self.get(url, **self._set_request_timeout(kwargs))

    def _delete(self, url, **kwargs):
        return self.delete(url, **self._set_request_timeout(kwargs))

    def _url(self, path):
        return '{0}/v{1}{2}'.format(self.base_url, self._version, path)

    def _raise_for_status(self, response, explanation=None):
        """Raises stored :class:`APIError`, if one occurred."""
        try:
            response.raise_for_status()
        except requests.exceptions.HTTPError as e:
            raise errors.APIError(e, response, explanation=explanation)

    def _result(self, response, json=False, binary=False):
        assert not (json and binary)
        self._raise_for_status(response)

        if json:
            return response.json()
        if binary:
            return response.content
        return response.text

    def _container_config(self, image, command, hostname=None, user=None,
                          detach=False, stdin_open=False, tty=False,
                          mem_limit=0, ports=None, environment=None, dns=None,
                          volumes=None, volumes_from=None,
                          network_disabled=False, entrypoint=None,
                          cpu_shares=None, working_dir=None, domainname=None,
                          memswap_limit=0):
        if isinstance(command, six.string_types):
            command = shlex.split(str(command))
        if isinstance(environment, dict):
            environment = [
                '{0}={1}'.format(k, v) for k, v in environment.items()
            ]

        if isinstance(ports, list):
            exposed_ports = {}
            for port_definition in ports:
                port = port_definition
                proto = 'tcp'
                if isinstance(port_definition, tuple):
                    if len(port_definition) == 2:
                        proto = port_definition[1]
                    port = port_definition[0]
                exposed_ports['{0}/{1}'.format(port, proto)] = {}
            ports = exposed_ports

        if isinstance(volumes, list):
            volumes_dict = {}
            for vol in volumes:
                volumes_dict[vol] = {}
            volumes = volumes_dict

        if volumes_from:
            if not isinstance(volumes_from, six.string_types):
                volumes_from = ','.join(volumes_from)
        else:
            # Force None, an empty list or dict causes client.start to fail
            volumes_from = None

        attach_stdin = False
        attach_stdout = False
        attach_stderr = False
        stdin_once = False

        if not detach:
            attach_stdout = True
            attach_stderr = True

            if stdin_open:
                attach_stdin = True
                stdin_once = True

        if utils.compare_version('1.10', self._version) >= 0:
            message = ('{0!r} parameter has no effect on create_container().'
                       ' It has been moved to start()')
            if dns is not None:
                raise errors.DockerException(message.format('dns'))
            if volumes_from is not None:
                raise errors.DockerException(message.format('volumes_from'))

        return {
            'Hostname': hostname,
            'Domainname': domainname,
            'ExposedPorts': ports,
            'User': user,
            'Tty': tty,
            'OpenStdin': stdin_open,
            'StdinOnce': stdin_once,
            'Memory': mem_limit,
            'AttachStdin': attach_stdin,
            'AttachStdout': attach_stdout,
            'AttachStderr': attach_stderr,
            'Env': environment,
            'Cmd': command,
            'Dns': dns,
            'Image': image,
            'Volumes': volumes,
            'VolumesFrom': volumes_from,
            'NetworkDisabled': network_disabled,
            'Entrypoint': entrypoint,
            'CpuShares': cpu_shares,
            'WorkingDir': working_dir,
            'MemorySwap': memswap_limit
        }

    def _post_json(self, url, data, **kwargs):
        # Go <1.1 can't unserialize null to a string
        # so we do this disgusting thing here.
        data2 = {}
        if data is not None:
            for k, v in six.iteritems(data):
                if v is not None:
                    data2[k] = v

        if 'headers' not in kwargs:
            kwargs['headers'] = {}
        kwargs['headers']['Content-Type'] = 'application/json'
        return self._post(url, data=json.dumps(data2), **kwargs)

    def _attach_params(self, override=None):
        return override or {
            'stdout': 1,
            'stderr': 1,
            'stream': 1
        }

    def _attach_websocket(self, container, params=None):
        if six.PY3:
            raise NotImplementedError("This method is not currently supported "
                                      "under python 3")
        url = self._url("/containers/{0}/attach/ws".format(container))
        req = requests.Request("POST", url, params=self._attach_params(params))
        full_url = req.prepare().url
        full_url = full_url.replace("http://", "ws://", 1)
        full_url = full_url.replace("https://", "wss://", 1)
        return self._create_websocket_connection(full_url)

    def _create_websocket_connection(self, url):
        return websocket.create_connection(url)

    def _get_raw_response_socket(self, response):
        self._raise_for_status(response)
        if six.PY3:
            return response.raw._fp.fp.raw._sock
        else:
            return response.raw._fp.fp._sock

    def _stream_helper(self, response):
        """Generator for data coming from a chunked-encoded HTTP response."""
        socket_fp = self._get_raw_response_socket(response)
        socket_fp.setblocking(1)
        socket = socket_fp.makefile()
        while True:
            # Because Docker introduced newlines at the end of chunks in v0.9,
            # and only on some API endpoints, we have to cater for both cases.
            size_line = socket.readline()
            if size_line == '\r\n':
                size_line = socket.readline()

            size = int(size_line, 16)
            if size <= 0:
                break
            data = socket.readline()
            if not data:
                break
            yield data

    def _multiplexed_buffer_helper(self, response):
        """A generator of multiplexed data blocks read from a buffered
        response."""
        buf = self._result(response, binary=True)
        walker = 0
        while True:
            if len(buf[walker:]) < 8:
                break
            _, length = struct.unpack_from('>BxxxL', buf[walker:])
            start = walker + STREAM_HEADER_SIZE_BYTES
            end = start + length
            walker = end
            yield buf[start:end]

    def _multiplexed_socket_stream_helper(self, response):
        """A generator of multiplexed data blocks coming from a response
        socket."""
        socket = self._get_raw_response_socket(response)

        def recvall(socket, size):
            blocks = []
            while size > 0:
                block = socket.recv(size)
                if not block:
                    return None

                blocks.append(block)
                size -= len(block)

            sep = bytes() if six.PY3 else str()
            data = sep.join(blocks)
            return data

        while True:
            socket.settimeout(None)
            header = recvall(socket, STREAM_HEADER_SIZE_BYTES)
            if not header:
                break
            _, length = struct.unpack('>BxxxL', header)
            if not length:
                break
            data = recvall(socket, length)
            if not data:
                break
            yield data

    def attach(self, container, stdout=True, stderr=True,
               stream=False, logs=False):
        if isinstance(container, dict):
            container = container.get('Id')
        params = {
            'logs': logs and 1 or 0,
            'stdout': stdout and 1 or 0,
            'stderr': stderr and 1 or 0,
            'stream': stream and 1 or 0,
        }
        u = self._url("/containers/{0}/attach".format(container))
        response = self._post(u, params=params, stream=stream)

        # Stream multi-plexing was only introduced in API v1.6. Anything before
        # that needs old-style streaming.
        if utils.compare_version('1.6', self._version) < 0:
            def stream_result():
                self._raise_for_status(response)
                for line in response.iter_lines(chunk_size=1,
                                                decode_unicode=True):
                    # filter out keep-alive new lines
                    if line:
                        yield line

            return stream_result() if stream else \
                self._result(response, binary=True)

        sep = bytes() if six.PY3 else str()

        return stream and self._multiplexed_socket_stream_helper(response) or \
            sep.join([x for x in self._multiplexed_buffer_helper(response)])

    def attach_socket(self, container, params=None, ws=False):
        if params is None:
            params = {
                'stdout': 1,
                'stderr': 1,
                'stream': 1
            }

        if ws:
            return self._attach_websocket(container, params)

        if isinstance(container, dict):
            container = container.get('Id')

        u = self._url("/containers/{0}/attach".format(container))
        return self._get_raw_response_socket(self.post(
            u, None, params=self._attach_params(params), stream=True))

    def build(self, path=None, tag=None, quiet=False, fileobj=None,
              nocache=False, rm=False, stream=False, timeout=None,
              custom_context=False, encoding=None):
        remote = context = headers = None
        if path is None and fileobj is None:
            raise TypeError("Either path or fileobj needs to be provided.")

        if custom_context:
            if not fileobj:
                raise TypeError("You must specify fileobj with custom_context")
            context = fileobj
        elif fileobj is not None:
            context = utils.mkbuildcontext(fileobj)
        elif path.startswith(('http://', 'https://',
                              'git://', 'github.com/')):
            remote = path
        else:
            context = utils.tar(path)

        if utils.compare_version('1.8', self._version) >= 0:
            stream = True

        u = self._url('/build')
        params = {
            't': tag,
            'remote': remote,
            'q': quiet,
            'nocache': nocache,
            'rm': rm
        }

        if context is not None:
            headers = {'Content-Type': 'application/tar'}
            if encoding:
                headers['Content-Encoding'] = encoding

        if utils.compare_version('1.9', self._version) >= 0:
            # If we don't have any auth data so far, try reloading the config
            # file one more time in case anything showed up in there.
            if not self._auth_configs:
                self._auth_configs = auth.load_config()

            # Send the full auth configuration (if any exists), since the build
            # could use any (or all) of the registries.
            if self._auth_configs:
                headers['X-Registry-Config'] = auth.encode_full_header(
                    self._auth_configs
                )

        response = self._post(
            u,
            data=context,
            params=params,
            headers=headers,
            stream=stream,
            timeout=timeout,
        )

        if context is not None:
            context.close()

        if stream:
            return self._stream_helper(response)
        else:
            output = self._result(response)
            srch = r'Successfully built ([0-9a-f]+)'
            match = re.search(srch, output)
            if not match:
                return None, output
            return match.group(1), output

    def commit(self, container, repository=None, tag=None, message=None,
               author=None, conf=None):
        params = {
            'container': container,
            'repo': repository,
            'tag': tag,
            'comment': message,
            'author': author
        }
        u = self._url("/commit")
        return self._result(self._post_json(u, data=conf, params=params),
                            json=True)

    def containers(self, quiet=False, all=False, trunc=True, latest=False,
                   since=None, before=None, limit=-1, size=False):
        params = {
            'limit': 1 if latest else limit,
            'all': 1 if all else 0,
            'size': 1 if size else 0,
            'trunc_cmd': 1 if trunc else 0,
            'since': since,
            'before': before
        }
        u = self._url("/containers/json")
        res = self._result(self._get(u, params=params), True)

        if quiet:
            return [{'Id': x['Id']} for x in res]
        return res

    def copy(self, container, resource):
        if isinstance(container, dict):
            container = container.get('Id')
        res = self._post_json(
            self._url("/containers/{0}/copy".format(container)),
            data={"Resource": resource},
            stream=True
        )
        self._raise_for_status(res)
        return res.raw

    def create_container(self, image, command=None, hostname=None, user=None,
                         detach=False, stdin_open=False, tty=False,
                         mem_limit=0, ports=None, environment=None, dns=None,
                         volumes=None, volumes_from=None,
                         network_disabled=False, name=None, entrypoint=None,
                         cpu_shares=None, working_dir=None, domainname=None,
                         memswap_limit=0):

        config = self._container_config(
            image, command, hostname, user, detach, stdin_open, tty, mem_limit,
            ports, environment, dns, volumes, volumes_from, network_disabled,
            entrypoint, cpu_shares, working_dir, domainname, memswap_limit
        )
        return self.create_container_from_config(config, name)

    def create_container_from_config(self, config, name=None):
        u = self._url("/containers/create")
        params = {
            'name': name
        }
        res = self._post_json(u, data=config, params=params)
        return self._result(res, True)

    def diff(self, container):
        if isinstance(container, dict):
            container = container.get('Id')
        return self._result(self._get(self._url("/containers/{0}/changes".
                            format(container))), True)

    def events(self):
        return self._stream_helper(self.get(self._url('/events'), stream=True))

    def export(self, container):
        if isinstance(container, dict):
            container = container.get('Id')
        res = self._get(self._url("/containers/{0}/export".format(container)),
                        stream=True)
        self._raise_for_status(res)
        return res.raw

    def get_image(self, image):
        res = self._get(self._url("/images/{0}/get".format(image)),
                        stream=True)
        self._raise_for_status(res)
        return res.raw

    def history(self, image):
        res = self._get(self._url("/images/{0}/history".format(image)))
        self._raise_for_status(res)
        return self._result(res)

    def images(self, name=None, quiet=False, all=False, viz=False):
        if viz:
            if utils.compare_version('1.7', self._version) >= 0:
                raise Exception('Viz output is not supported in API >= 1.7!')
            return self._result(self._get(self._url("images/viz")))
        params = {
            'filter': name,
            'only_ids': 1 if quiet else 0,
            'all': 1 if all else 0,
        }
        res = self._result(self._get(self._url("/images/json"), params=params),
                           True)
        if quiet:
            return [x['Id'] for x in res]
        return res

    def import_image(self, src=None, repository=None, tag=None, image=None):
        u = self._url("/images/create")
        params = {
            'repo': repository,
            'tag': tag
        }

        if src:
            try:
                # XXX: this is ways not optimal but the only way
                # for now to import tarballs through the API
                fic = open(src)
                data = fic.read()
                fic.close()
                src = "-"
            except IOError:
                # file does not exists or not a file (URL)
                data = None
            if isinstance(src, six.string_types):
                params['fromSrc'] = src
                return self._result(self._post(u, data=data, params=params))
            return self._result(self._post(u, data=src, params=params))

        if image:
            params['fromImage'] = image
            return self._result(self._post(u, data=None, params=params))

        raise Exception("Must specify a src or image")

    def info(self):
        return self._result(self._get(self._url("/info")),
                            True)

    def insert(self, image, url, path):
        if utils.compare_version('1.12', self._version) >= 0:
            raise errors.DeprecatedMethod(
                'insert is not available for API version >=1.12'
            )
        api_url = self._url("/images/" + image + "/insert")
        params = {
            'url': url,
            'path': path
        }
        return self._result(self._post(api_url, params=params))

    def inspect_container(self, container):
        if isinstance(container, dict):
            container = container.get('Id')
        return self._result(
            self._get(self._url("/containers/{0}/json".format(container))),
            True)

    def inspect_image(self, image_id):
        return self._result(
            self._get(self._url("/images/{0}/json".format(image_id))),
            True
        )

    def kill(self, container, signal=None):
        if isinstance(container, dict):
            container = container.get('Id')
        url = self._url("/containers/{0}/kill".format(container))
        params = {}
        if signal is not None:
            params['signal'] = signal
        res = self._post(url, params=params)

        self._raise_for_status(res)

    def load_image(self, data):
        res = self._post(self._url("/images/load"), data=data)
        self._raise_for_status(res)

    def login(self, username, password=None, email=None, registry=None,
              reauth=False):
        # If we don't have any auth data so far, try reloading the config file
        # one more time in case anything showed up in there.
        if not self._auth_configs:
            self._auth_configs = auth.load_config()

        registry = registry or auth.INDEX_URL

        authcfg = auth.resolve_authconfig(self._auth_configs, registry)
        # If we found an existing auth config for this registry and username
        # combination, we can return it immediately unless reauth is requested.
        if authcfg and authcfg.get('username', None) == username \
                and not reauth:
            return authcfg

        req_data = {
            'username': username,
            'password': password,
            'email': email,
            'serveraddress': registry,
        }

        response = self._post_json(self._url('/auth'), data=req_data)
        if response.status_code == 200:
            self._auth_configs[registry] = req_data
        return self._result(response, json=True)

    def logs(self, container, stdout=True, stderr=True, stream=False,
             timestamps=False):
        if isinstance(container, dict):
            container = container.get('Id')
        if utils.compare_version('1.11', self._version) >= 0:
            params = {'stderr': stderr and 1 or 0,
                      'stdout': stdout and 1 or 0,
                      'timestamps': timestamps and 1 or 0,
                      'follow': stream and 1 or 0}
            url = self._url("/containers/{0}/logs".format(container))
            res = self._get(url, params=params, stream=stream)
            if stream:
                return self._multiplexed_socket_stream_helper(res)
            elif six.PY3:
                return bytes().join(
                    [x for x in self._multiplexed_buffer_helper(res)]
                )
            else:
                return str().join(
                    [x for x in self._multiplexed_buffer_helper(res)]
                )
        return self.attach(
            container,
            stdout=stdout,
            stderr=stderr,
            stream=stream,
            logs=True
        )

    def ping(self):
        return self._result(self._get(self._url('/_ping')))

    def port(self, container, private_port):
        if isinstance(container, dict):
            container = container.get('Id')
        res = self._get(self._url("/containers/{0}/json".format(container)))
        self._raise_for_status(res)
        json_ = res.json()
        s_port = str(private_port)
        h_ports = None

        h_ports = json_['NetworkSettings']['Ports'].get(s_port + '/udp')
        if h_ports is None:
            h_ports = json_['NetworkSettings']['Ports'].get(s_port + '/tcp')

        return h_ports

    def pull(self, repository, tag=None, stream=False):
        if not tag:
            repository, tag = utils.parse_repository_tag(repository)
        registry, repo_name = auth.resolve_repository_name(repository)
        if repo_name.count(":") == 1:
            repository, tag = repository.rsplit(":", 1)

        params = {
            'tag': tag,
            'fromImage': repository
        }
        headers = {}

        if utils.compare_version('1.5', self._version) >= 0:
            # If we don't have any auth data so far, try reloading the config
            # file one more time in case anything showed up in there.
            if not self._auth_configs:
                self._auth_configs = auth.load_config()
            authcfg = auth.resolve_authconfig(self._auth_configs, registry)

            # Do not fail here if no authentication exists for this specific
            # registry as we can have a readonly pull. Just put the header if
            # we can.
            if authcfg:
                headers['X-Registry-Auth'] = auth.encode_header(authcfg)

        response = self._post(self._url('/images/create'), params=params,
                              headers=headers, stream=stream, timeout=None)

        if stream:
            return self._stream_helper(response)
        else:
            return self._result(response)

    def push(self, repository, stream=False):
        registry, repo_name = auth.resolve_repository_name(repository)
        u = self._url("/images/{0}/push".format(repository))
        headers = {}

        if utils.compare_version('1.5', self._version) >= 0:
            # If we don't have any auth data so far, try reloading the config
            # file one more time in case anything showed up in there.
            if not self._auth_configs:
                self._auth_configs = auth.load_config()
            authcfg = auth.resolve_authconfig(self._auth_configs, registry)

            # Do not fail here if no authentication exists for this specific
            # registry as we can have a readonly pull. Just put the header if
            # we can.
            if authcfg:
                headers['X-Registry-Auth'] = auth.encode_header(authcfg)

            response = self._post_json(u, None, headers=headers, stream=stream)
        else:
            response = self._post_json(u, None, stream=stream)

        return stream and self._stream_helper(response) \
            or self._result(response)

    def remove_container(self, container, v=False, link=False, force=False):
        if isinstance(container, dict):
            container = container.get('Id')
        params = {'v': v, 'link': link, 'force': force}
        res = self._delete(self._url("/containers/" + container),
                           params=params)
        self._raise_for_status(res)

    def remove_image(self, image, force=False, noprune=False):
        params = {'force': force, 'noprune': noprune}
        res = self._delete(self._url("/images/" + image), params=params)
        self._raise_for_status(res)

    def restart(self, container, timeout=10):
        if isinstance(container, dict):
            container = container.get('Id')
        params = {'t': timeout}
        url = self._url("/containers/{0}/restart".format(container))
        res = self._post(url, params=params)
        self._raise_for_status(res)

    def search(self, term):
        return self._result(self._get(self._url("/images/search"),
                                      params={'term': term}),
                            True)

    def start(self, container, binds=None, port_bindings=None, lxc_conf=None,
              publish_all_ports=False, links=None, privileged=False,
              dns=None, dns_search=None, volumes_from=None, network_mode=None):
        if isinstance(container, dict):
            container = container.get('Id')

        if isinstance(lxc_conf, dict):
            formatted = []
            for k, v in six.iteritems(lxc_conf):
                formatted.append({'Key': k, 'Value': str(v)})
            lxc_conf = formatted

        start_config = {
            'LxcConf': lxc_conf
        }
        if binds:
            start_config['Binds'] = utils.convert_volume_binds(binds)

        if port_bindings:
            start_config['PortBindings'] = utils.convert_port_bindings(
                port_bindings
            )

        start_config['PublishAllPorts'] = publish_all_ports

        if links:
            if isinstance(links, dict):
                links = six.iteritems(links)

            formatted_links = [
                '{0}:{1}'.format(k, v) for k, v in sorted(links)
            ]

            start_config['Links'] = formatted_links

        start_config['Privileged'] = privileged

        if utils.compare_version('1.10', self._version) >= 0:
            if dns is not None:
                start_config['Dns'] = dns
            if volumes_from is not None:
                if isinstance(volumes_from, six.string_types):
                    volumes_from = volumes_from.split(',')
                start_config['VolumesFrom'] = volumes_from
        else:
            warning_message = ('{0!r} parameter is discarded. It is only'
                               ' available for API version greater or equal'
                               ' than 1.10')

            if dns is not None:
                warnings.warn(warning_message.format('dns'),
                              DeprecationWarning)
            if volumes_from is not None:
                warnings.warn(warning_message.format('volumes_from'),
                              DeprecationWarning)

        if dns_search:
            start_config['DnsSearch'] = dns_search

        if network_mode:
            start_config['NetworkMode'] = network_mode

        url = self._url("/containers/{0}/start".format(container))
        res = self._post_json(url, data=start_config)
        self._raise_for_status(res)

    def resize(self, container, height, width):
        if isinstance(container, dict):
            container = container.get('Id')

        params = {'h': height, 'w': width}
        url = self._url("/containers/{0}/resize".format(container))
        res = self._post(url, params=params)
        self._raise_for_status(res)

    def stop(self, container, timeout=10):
        if isinstance(container, dict):
            container = container.get('Id')
        params = {'t': timeout}
        url = self._url("/containers/{0}/stop".format(container))
        res = self._post(url, params=params,
                         timeout=max(timeout, self._timeout))
        self._raise_for_status(res)

    def tag(self, image, repository, tag=None, force=False):
        params = {
            'tag': tag,
            'repo': repository,
            'force': 1 if force else 0
        }
        url = self._url("/images/{0}/tag".format(image))
        res = self._post(url, params=params)
        self._raise_for_status(res)
        return res.status_code == 201

    def top(self, container):
        u = self._url("/containers/{0}/top".format(container))
        return self._result(self._get(u), True)

    def version(self):
        return self._result(self._get(self._url("/version")), True)

    def wait(self, container):
        if isinstance(container, dict):
            container = container.get('Id')
        url = self._url("/containers/{0}/wait".format(container))
        res = self._post(url, timeout=None)
        self._raise_for_status(res)
        json_ = res.json()
        if 'StatusCode' in json_:
            return json_['StatusCode']
        return -1
