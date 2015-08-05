from __future__ import unicode_literals
from __future__ import absolute_import
from collections import namedtuple
import logging
import os
import re
import os
import sys
from operator import attrgetter

import six
from docker.errors import APIError
from docker.utils import create_host_config, LogConfig

from . import __version__
from .config import DOCKER_CONFIG_KEYS, merge_environment
from .const import (
    DEFAULT_TIMEOUT,
    LABEL_CONTAINER_NUMBER,
    LABEL_ONE_OFF,
    LABEL_PROJECT,
    LABEL_SERVICE,
    LABEL_VERSION,
    LABEL_CONFIG_HASH,
)
from .container import Container
from .legacy import check_for_legacy_containers
from .progress_stream import stream_output, StreamOutputError
from .utils import json_hash, parallel_execute

log = logging.getLogger(__name__)


DOCKER_START_KEYS = [
    'cap_add',
    'cap_drop',
    'devices',
    'dns',
    'dns_search',
    'env_file',
    'extra_hosts',
    'read_only',
    'net',
    'log_driver',
    'log_opt',
    'pid',
    'privileged',
    'restart',
    'volumes_from',
    'security_opt',
]

VALID_NAME_CHARS = '[a-zA-Z0-9\._\-]'


class BuildError(Exception):
    def __init__(self, service, reason):
        self.service = service
        self.reason = reason


class ConfigError(ValueError):
    pass


class NeedsBuildError(Exception):
    def __init__(self, service):
        self.service = service


class NoSuchImageError(Exception):
    pass


VolumeSpec = namedtuple('VolumeSpec', 'external internal mode')


ServiceName = namedtuple('ServiceName', 'project service number')


ConvergencePlan = namedtuple('ConvergencePlan', 'action containers')


class Service(object):
    def __init__(self, name, client=None, project='default', links=None, external_links=None, volumes_from=None, net=None, **options):
        if not re.match('^%s+$' % VALID_NAME_CHARS, name):
            raise ConfigError('Invalid service name "%s" - only %s are allowed' % (name, VALID_NAME_CHARS))
        if not re.match('^%s+$' % VALID_NAME_CHARS, project):
            raise ConfigError('Invalid project name "%s" - only %s are allowed' % (project, VALID_NAME_CHARS))
        if 'image' in options and 'build' in options:
            raise ConfigError('Service %s has both an image and build path specified. A service can either be built to image or use an existing image, not both.' % name)
        if 'image' not in options and 'build' not in options:
            raise ConfigError('Service %s has neither an image nor a build path specified. Exactly one must be provided.' % name)

        self.name = name
        self.client = client
        self.project = project
        self.links = links or []
        self.external_links = external_links or []
        self.volumes_from = volumes_from or []
        self.net = net or None
        self.options = options

    def containers(self, stopped=False, one_off=False):
        containers = [
            Container.from_ps(self.client, container)
            for container in self.client.containers(
                all=stopped,
                filters={'label': self.labels(one_off=one_off)})]

        if not containers:
            check_for_legacy_containers(
                self.client,
                self.project,
                [self.name],
            )

        return containers

    def get_container(self, number=1):
        """Return a :class:`compose.container.Container` for this service. The
        container must be active, and match `number`.
        """
        labels = self.labels() + ['{0}={1}'.format(LABEL_CONTAINER_NUMBER, number)]
        for container in self.client.containers(filters={'label': labels}):
            return Container.from_ps(self.client, container)

        raise ValueError("No container found for %s_%s" % (self.name, number))

    def start(self, **options):
        for c in self.containers(stopped=True):
            self.start_container_if_stopped(c, **options)

    # TODO: remove these functions, project takes care of starting/stopping,
    def stop(self, **options):
        for c in self.containers():
            log.info("Stopping %s..." % c.name)
            c.stop(**options)

    def kill(self, **options):
        for c in self.containers():
            log.info("Killing %s..." % c.name)
            c.kill(**options)

    def restart(self, **options):
        for c in self.containers():
            log.info("Restarting %s..." % c.name)
            c.restart(**options)

    # end TODO

    def scale(self, desired_num, timeout=DEFAULT_TIMEOUT):
        """
        Adjusts the number of containers to the specified number and ensures
        they are running.

        - creates containers until there are at least `desired_num`
        - stops containers until there are at most `desired_num` running
        - starts containers until there are at least `desired_num` running
        - removes all stopped containers
        """
        if self.custom_container_name() and desired_num > 1:
            log.warn('The "%s" service is using the custom container name "%s". '
                     'Docker requires each container to have a unique name. '
                     'Remove the custom name to scale the service.'
                     % (self.name, self.custom_container_name()))

        if self.specifies_host_port():
            log.warn('The "%s" service specifies a port on the host. If multiple containers '
                     'for this service are created on a single host, the port will clash.'
                     % self.name)

        def create_and_start(service, number):
            container = service.create_container(number=number, quiet=True)
            container.start()
            return container

        running_containers = self.containers(stopped=False)
        num_running = len(running_containers)

        if desired_num == num_running:
            # do nothing as we already have the desired number
            log.info('Desired container number already achieved')
            return

        if desired_num > num_running:
            # we need to start/create until we have desired_num
            all_containers = self.containers(stopped=True)

            if num_running != len(all_containers):
                # we have some stopped containers, let's start them up again
                stopped_containers = sorted([c for c in all_containers if not c.is_running], key=attrgetter('number'))

                num_stopped = len(stopped_containers)

                if num_stopped + num_running > desired_num:
                    num_to_start = desired_num - num_running
                    containers_to_start = stopped_containers[:num_to_start]
                else:
                    containers_to_start = stopped_containers

                parallel_execute(
                    objects=containers_to_start,
                    obj_callable=lambda c: c.start(),
                    msg_index=lambda c: c.name,
                    msg="Starting"
                )

                num_running += len(containers_to_start)

            num_to_create = desired_num - num_running
            next_number = self._next_container_number()
            container_numbers = [
                number for number in range(
                    next_number, next_number + num_to_create
                )
            ]

            parallel_execute(
                objects=container_numbers,
                obj_callable=lambda n: create_and_start(service=self, number=n),
                msg_index=lambda n: n,
                msg="Creating and starting"
            )

        if desired_num < num_running:
            num_to_stop = num_running - desired_num
            sorted_running_containers = sorted(running_containers, key=attrgetter('number'))
            containers_to_stop = sorted_running_containers[-num_to_stop:]

            parallel_execute(
                objects=containers_to_stop,
                obj_callable=lambda c: c.stop(timeout=timeout),
                msg_index=lambda c: c.name,
                msg="Stopping"
            )

        self.remove_stopped()

    def remove_stopped(self, **options):
        containers = [c for c in self.containers(stopped=True) if not c.is_running]

        parallel_execute(
            objects=containers,
            obj_callable=lambda c: c.remove(**options),
            msg_index=lambda c: c.name,
            msg="Removing"
        )

    def create_container(self,
                         one_off=False,
                         do_build=True,
                         previous_container=None,
                         number=None,
                         quiet=False,
                         **override_options):
        """
        Create a container for this service. If the image doesn't exist, attempt to pull
        it.
        """
        self.ensure_image_exists(
            do_build=do_build,
        )

        container_options = self._get_container_create_options(
            override_options,
            number or self._next_container_number(one_off=one_off),
            one_off=one_off,
            previous_container=previous_container,
        )

        if 'name' in container_options and not quiet:
            log.info("Creating %s..." % container_options['name'])

        return Container.create(self.client, **container_options)

    def ensure_image_exists(self,
                            do_build=True):

        try:
            self.image()
            return
        except NoSuchImageError:
            pass

        if self.can_be_built():
            if do_build:
                self.build()
            else:
                raise NeedsBuildError(self)

        elif self.can_be_loaded():
            with open(self.options['load_image'], 'r') as data:
                self.client.load_image(data=data)
        else:
            self.pull()

    def image(self):
        try:
            return self.client.inspect_image(self.image_name)
        except APIError as e:
            if e.response.status_code == 404 and e.explanation and 'No such image' in str(e.explanation):
                raise NoSuchImageError("Image '{}' not found".format(self.image_name))
            else:
                raise

    @property
    def image_name(self):
        if self.can_be_built():
            return self.full_name
        else:
            return self.options['image']

    def convergence_plan(self,
                         allow_recreate=True,
                         force_recreate=False):

        if force_recreate and not allow_recreate:
            raise ValueError("force_recreate and allow_recreate are in conflict")

        containers = self.containers(stopped=True)

        if not containers:
            return ConvergencePlan('create', [])

        if not allow_recreate:
            return ConvergencePlan('start', containers)

        if force_recreate or self._containers_have_diverged(containers):
            return ConvergencePlan('recreate', containers)

        stopped = [c for c in containers if not c.is_running]

        if stopped:
            return ConvergencePlan('start', stopped)

        return ConvergencePlan('noop', containers)

    def _containers_have_diverged(self, containers):
        config_hash = None

        try:
            config_hash = self.config_hash()
        except NoSuchImageError as e:
            log.debug(
                'Service %s has diverged: %s',
                self.name, six.text_type(e),
            )
            return True

        has_diverged = False

        for c in containers:
            container_config_hash = c.labels.get(LABEL_CONFIG_HASH, None)
            if container_config_hash != config_hash:
                log.debug(
                    '%s has diverged: %s != %s',
                    c.name, container_config_hash, config_hash,
                )
                has_diverged = True

        return has_diverged

    def execute_convergence_plan(self,
                                 plan,
                                 do_build=True,
                                 timeout=DEFAULT_TIMEOUT):
        (action, containers) = plan

        if action == 'create':
            container = self.create_container(
                do_build=do_build,
            )
            self.start_container(container)

            return [container]

        elif action == 'recreate':
            return [
                self.recreate_container(
                    c,
                    timeout=timeout
                )
                for c in containers
            ]

        elif action == 'start':
            for c in containers:
                self.start_container_if_stopped(c)

            return containers

        elif action == 'noop':
            for c in containers:
                log.info("%s is up-to-date" % c.name)

            return containers

        else:
            raise Exception("Invalid action: {}".format(action))

    def recreate_container(self,
                           container,
                           timeout=DEFAULT_TIMEOUT):
        """Recreate a container.

        The original container is renamed to a temporary name so that data
        volumes can be copied to the new container, before the original
        container is removed.
        """
        log.info("Recreating %s..." % container.name)
        try:
            container.stop(timeout=timeout)
        except APIError as e:
            if (e.response.status_code == 500
                    and e.explanation
                    and 'no such process' in str(e.explanation)):
                pass
            else:
                raise

        # Use a hopefully unique container name by prepending the short id
        self.client.rename(
            container.id,
            '%s_%s' % (container.short_id, container.name))

        new_container = self.create_container(
            do_build=False,
            previous_container=container,
            number=container.labels.get(LABEL_CONTAINER_NUMBER),
            quiet=True,
        )
        self.start_container(new_container)
        container.remove()
        return new_container

    def start_container_if_stopped(self, container):
        if container.is_running:
            return container
        else:
            log.info("Starting %s..." % container.name)
            return self.start_container(container)

    def start_container(self, container):
        container.start()
        return container

    def remove_duplicate_containers(self, timeout=DEFAULT_TIMEOUT):
        for c in self.duplicate_containers():
            log.info('Removing %s...' % c.name)
            c.stop(timeout=timeout)
            c.remove()

    def duplicate_containers(self):
        containers = sorted(
            self.containers(stopped=True),
            key=lambda c: c.get('Created'),
        )

        numbers = set()

        for c in containers:
            if c.number in numbers:
                yield c
            else:
                numbers.add(c.number)

    def config_hash(self):
        return json_hash(self.config_dict())

    def config_dict(self):
        return {
            'options': self.options,
            'image_id': self.image()['Id'],
        }

    def get_dependency_names(self):
        net_name = self.get_net_name()
        return (self.get_linked_names() +
                self.get_volumes_from_names() +
                ([net_name] if net_name else []))

    def get_linked_names(self):
        return [s.name for (s, _) in self.links]

    def get_volumes_from_names(self):
        return [s.name for s in self.volumes_from if isinstance(s, Service)]

    def get_net_name(self):
        if isinstance(self.net, Service):
            return self.net.name
        else:
            return

    def get_container_name(self, number, one_off=False):
        # TODO: Implement issue #652 here
        return build_container_name(self.project, self.name, number, one_off)

    # TODO: this would benefit from github.com/docker/docker/pull/11943
    # to remove the need to inspect every container
    def _next_container_number(self, one_off=False):
        numbers = [
            Container.from_ps(self.client, container).number
            for container in self.client.containers(
                all=True,
                filters={'label': self.labels(one_off=one_off)})
        ]
        return 1 if not numbers else max(numbers) + 1

    def _get_links(self, link_to_self):
        links = []
        for service, link_name in self.links:
            for container in service.containers():
                links.append((container.name, link_name or service.name))
                links.append((container.name, container.name))
                links.append((container.name, container.name_without_project))
        if link_to_self:
            for container in self.containers():
                links.append((container.name, self.name))
                links.append((container.name, container.name))
                links.append((container.name, container.name_without_project))
        for external_link in self.external_links:
            if ':' not in external_link:
                link_name = external_link
            else:
                external_link, link_name = external_link.split(':')
            links.append((external_link, link_name))
        return links

    def _get_volumes_from(self):
        volumes_from = []
        for volume_source in self.volumes_from:
            if isinstance(volume_source, Service):
                containers = volume_source.containers(stopped=True)
                if not containers:
                    volumes_from.append(volume_source.create_container().id)
                else:
                    volumes_from.extend(map(attrgetter('id'), containers))

            elif isinstance(volume_source, Container):
                volumes_from.append(volume_source.id)

        return volumes_from

    def _get_net(self):
        if not self.net:
            return None

        if isinstance(self.net, Service):
            containers = self.net.containers()
            if len(containers) > 0:
                net = 'container:' + containers[0].id
            else:
                log.warning("Warning: Service %s is trying to use reuse the network stack "
                            "of another service that is not running." % (self.net.name))
                net = None
        elif isinstance(self.net, Container):
            net = 'container:' + self.net.id
        else:
            net = self.net

        return net

    def _get_container_create_options(
            self,
            override_options,
            number,
            one_off=False,
            previous_container=None):

        add_config_hash = (not one_off and not override_options)

        container_options = dict(
            (k, self.options[k])
            for k in DOCKER_CONFIG_KEYS if k in self.options)
        container_options.update(override_options)

        if self.custom_container_name() and not one_off:
            container_options['name'] = self.custom_container_name()
        else:
            container_options['name'] = self.get_container_name(number, one_off)

        if add_config_hash:
            config_hash = self.config_hash()
            if 'labels' not in container_options:
                container_options['labels'] = {}
            container_options['labels'][LABEL_CONFIG_HASH] = config_hash
            log.debug("Added config hash: %s" % config_hash)

        if 'detach' not in container_options:
            container_options['detach'] = True

        # If a qualified hostname was given, split it into an
        # unqualified hostname and a domainname unless domainname
        # was also given explicitly. This matches the behavior of
        # the official Docker CLI in that scenario.
        if ('hostname' in container_options
                and 'domainname' not in container_options
                and '.' in container_options['hostname']):
            parts = container_options['hostname'].partition('.')
            container_options['hostname'] = parts[0]
            container_options['domainname'] = parts[2]

        if 'ports' in container_options or 'expose' in self.options:
            ports = []
            all_ports = container_options.get('ports', []) + self.options.get('expose', [])
            for port in all_ports:
                port = str(port)
                if ':' in port:
                    port = port.split(':')[-1]
                if '/' in port:
                    port = tuple(port.split('/'))
                ports.append(port)
            container_options['ports'] = ports

        override_options['binds'] = merge_volume_bindings(
            container_options.get('volumes') or [],
            previous_container)

        if 'volumes' in container_options:
            container_options['volumes'] = dict(
                (parse_volume_spec(v).internal, {})
                for v in container_options['volumes'])

        container_options['environment'] = merge_environment(
            self.options.get('environment'),
            override_options.get('environment'))

        if previous_container:
            container_options['environment']['affinity:container'] = ('=' + previous_container.id)

        container_options['image'] = self.image_name

        container_options['labels'] = build_container_labels(
            container_options.get('labels', {}),
            self.labels(one_off=one_off),
            number)

        # Delete options which are only used when starting
        for key in DOCKER_START_KEYS:
            container_options.pop(key, None)

        container_options['host_config'] = self._get_container_host_config(
            override_options,
            one_off=one_off)

        return container_options

    def _get_container_host_config(self, override_options, one_off=False):
        options = dict(self.options, **override_options)
        port_bindings = build_port_bindings(options.get('ports') or [])

        privileged = options.get('privileged', False)
        cap_add = options.get('cap_add', None)
        cap_drop = options.get('cap_drop', None)
        log_config = LogConfig(
            type=options.get('log_driver', 'json-file'),
            config=options.get('log_opt', None)
        )
        pid = options.get('pid', None)
        security_opt = options.get('security_opt', None)

        dns = options.get('dns', None)
        if isinstance(dns, six.string_types):
            dns = [dns]

        dns_search = options.get('dns_search', None)
        if isinstance(dns_search, six.string_types):
            dns_search = [dns_search]

        restart = parse_restart_spec(options.get('restart', None))

        extra_hosts = build_extra_hosts(options.get('extra_hosts', None))
        read_only = options.get('read_only', None)

        devices = options.get('devices', None)

        return create_host_config(
            links=self._get_links(link_to_self=one_off),
            port_bindings=port_bindings,
            binds=options.get('binds'),
            volumes_from=self._get_volumes_from(),
            privileged=privileged,
            network_mode=self._get_net(),
            devices=devices,
            dns=dns,
            dns_search=dns_search,
            restart_policy=restart,
            cap_add=cap_add,
            cap_drop=cap_drop,
            log_config=log_config,
            extra_hosts=extra_hosts,
            read_only=read_only,
            pid_mode=pid,
            security_opt=security_opt
        )

    def build(self, no_cache=False):
        log.info('Building %s...' % self.name)

        path = six.binary_type(self.options['build'])

        build_output = self.client.build(
            path=path,
            tag=self.image_name,
            stream=True,
            rm=True,
            pull=False,
            nocache=no_cache,
            dockerfile=self.options.get('dockerfile', None),
        )

        try:
            all_events = stream_output(build_output, sys.stdout)
        except StreamOutputError as e:
            raise BuildError(self, unicode(e))

        # Ensure the HTTP connection is not reused for another
        # streaming command, as the Docker daemon can sometimes
        # complain about it
        self.client.close()

        image_id = None

        for event in all_events:
            if 'stream' in event:
                match = re.search(r'Successfully built ([0-9a-f]+)', event.get('stream', ''))
                if match:
                    image_id = match.group(1)

        if image_id is None:
            raise BuildError(self, event if all_events else 'Unknown')

        return image_id

    def can_be_built(self):
        return 'build' in self.options

    def can_be_loaded(self):
        return 'load_image' in self.options and os.path.exists(self.options['load_image'])

    @property
    def full_name(self):
        """
        The tag to give to images built for this service.
        """
        return '%s_%s' % (self.project, self.name)

    def labels(self, one_off=False):
        return [
            '{0}={1}'.format(LABEL_PROJECT, self.project),
            '{0}={1}'.format(LABEL_SERVICE, self.name),
            '{0}={1}'.format(LABEL_ONE_OFF, "True" if one_off else "False")
        ]

    def custom_container_name(self):
        return self.options.get('container_name')

    def specifies_host_port(self):
        for port in self.options.get('ports', []):
            if ':' in str(port):
                return True
        return False

    def pull(self):
        if 'image' not in self.options:
            return

        repo, tag = parse_repository_tag(self.options['image'])
        tag = tag or 'latest'
        log.info('Pulling %s (%s:%s)...' % (self.name, repo, tag))
        output = self.client.pull(
            repo,
            tag=tag,
            stream=True,
        )
        stream_output(output, sys.stdout)


# Names


def build_container_name(project, service, number, one_off=False):
    bits = [project, service]
    if one_off:
        bits.append('run')
    return '_'.join(bits + [str(number)])


# Images


def parse_repository_tag(s):
    if ":" not in s:
        return s, ""
    repo, tag = s.rsplit(":", 1)
    if "/" in tag:
        return s, ""
    return repo, tag


# Volumes


def merge_volume_bindings(volumes_option, previous_container):
    """Return a list of volume bindings for a container. Container data volumes
    are replaced by those from the previous container.
    """
    volume_bindings = dict(
        build_volume_binding(parse_volume_spec(volume))
        for volume in volumes_option or []
        if ':' in volume)

    if previous_container:
        volume_bindings.update(
            get_container_data_volumes(previous_container, volumes_option))

    return volume_bindings.values()


def get_container_data_volumes(container, volumes_option):
    """Find the container data volumes that are in `volumes_option`, and return
    a mapping of volume bindings for those volumes.
    """
    volumes = []

    volumes_option = volumes_option or []
    container_volumes = container.get('Volumes') or {}
    image_volumes = container.image_config['ContainerConfig'].get('Volumes') or {}

    for volume in set(volumes_option + image_volumes.keys()):
        volume = parse_volume_spec(volume)
        # No need to preserve host volumes
        if volume.external:
            continue

        volume_path = container_volumes.get(volume.internal)
        # New volume, doesn't exist in the old container
        if not volume_path:
            continue

        # Copy existing volume from old container
        volume = volume._replace(external=volume_path)
        volumes.append(build_volume_binding(volume))

    return dict(volumes)


def build_volume_binding(volume_spec):
    return volume_spec.internal, "{}:{}:{}".format(*volume_spec)


def parse_volume_spec(volume_config):
    parts = volume_config.split(':')
    if len(parts) > 3:
        raise ConfigError("Volume %s has incorrect format, should be "
                          "external:internal[:mode]" % volume_config)

    if len(parts) == 1:
        external = None
        internal = os.path.normpath(parts[0])
    else:
        external = os.path.normpath(parts[0])
        internal = os.path.normpath(parts[1])

    mode = parts[2] if len(parts) == 3 else 'rw'

    return VolumeSpec(external, internal, mode)


# Ports


def build_port_bindings(ports):
    port_bindings = {}
    for port in ports:
        internal_port, external = split_port(port)
        if internal_port in port_bindings:
            port_bindings[internal_port].append(external)
        else:
            port_bindings[internal_port] = [external]
    return port_bindings


def split_port(port):
    parts = str(port).split(':')
    if not 1 <= len(parts) <= 3:
        raise ConfigError('Invalid port "%s", should be '
                          '[[remote_ip:]remote_port:]port[/protocol]' % port)

    if len(parts) == 1:
        internal_port, = parts
        return internal_port, None
    if len(parts) == 2:
        external_port, internal_port = parts
        return internal_port, external_port

    external_ip, external_port, internal_port = parts
    return internal_port, (external_ip, external_port or None)


# Labels


def build_container_labels(label_options, service_labels, number, one_off=False):
    labels = label_options or {}
    labels.update(label.split('=', 1) for label in service_labels)
    labels[LABEL_CONTAINER_NUMBER] = str(number)
    labels[LABEL_VERSION] = __version__
    return labels


# Restart policy


def parse_restart_spec(restart_config):
    if not restart_config:
        return None
    parts = restart_config.split(':')
    if len(parts) > 2:
        raise ConfigError("Restart %s has incorrect format, should be "
                          "mode[:max_retry]" % restart_config)
    if len(parts) == 2:
        name, max_retry_count = parts
    else:
        name, = parts
        max_retry_count = 0

    return {'Name': name, 'MaximumRetryCount': int(max_retry_count)}


# Extra hosts


def build_extra_hosts(extra_hosts_config):
    if not extra_hosts_config:
        return {}

    if isinstance(extra_hosts_config, list):
        extra_hosts_dict = {}
        for extra_hosts_line in extra_hosts_config:
            if not isinstance(extra_hosts_line, six.string_types):
                raise ConfigError(
                    "extra_hosts_config \"%s\" must be either a list of strings or a string->string mapping," %
                    extra_hosts_config
                )
            host, ip = extra_hosts_line.split(':')
            extra_hosts_dict.update({host.strip(): ip.strip()})
        extra_hosts_config = extra_hosts_dict

    if isinstance(extra_hosts_config, dict):
        return extra_hosts_config

    raise ConfigError(
        "extra_hosts_config \"%s\" must be either a list of strings or a string->string mapping," %
        extra_hosts_config
    )
