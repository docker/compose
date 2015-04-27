from __future__ import unicode_literals
from __future__ import absolute_import
from collections import namedtuple
import logging
import re
from operator import attrgetter
import sys
import six

from docker.errors import APIError
from docker.utils import create_host_config

from .config import DOCKER_CONFIG_KEYS
from .container import Container, get_container_name
from .progress_stream import stream_output, StreamOutputError

log = logging.getLogger(__name__)


DOCKER_START_KEYS = [
    'cap_add',
    'cap_drop',
    'dns',
    'dns_search',
    'env_file',
    'extra_hosts',
    'net',
    'pid',
    'privileged',
    'restart',
]

VALID_NAME_CHARS = '[a-zA-Z0-9]'


class BuildError(Exception):
    def __init__(self, service, reason):
        self.service = service
        self.reason = reason


class CannotBeScaledError(Exception):
    pass


class ConfigError(ValueError):
    pass


VolumeSpec = namedtuple('VolumeSpec', 'external internal mode')


ServiceName = namedtuple('ServiceName', 'project service number')


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
        return [Container.from_ps(self.client, container)
                for container in self.client.containers(all=stopped)
                if self.has_container(container, one_off=one_off)]

    def has_container(self, container, one_off=False):
        """Return True if `container` was created to fulfill this service."""
        name = get_container_name(container)
        if not name or not is_valid_name(name, one_off):
            return False
        project, name, _number = parse_name(name)
        return project == self.project and name == self.name

    def get_container(self, number=1):
        """Return a :class:`compose.container.Container` for this service. The
        container must be active, and match `number`.
        """
        for container in self.client.containers():
            if not self.has_container(container):
                continue
            _, _, container_number = parse_name(get_container_name(container))
            if container_number == number:
                return Container.from_ps(self.client, container)

        raise ValueError("No container found for %s_%s" % (self.name, number))

    def start(self, **options):
        for c in self.containers(stopped=True):
            self.start_container_if_stopped(c, **options)

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

    def scale(self, desired_num):
        """
        Adjusts the number of containers to the specified number and ensures
        they are running.

        - creates containers until there are at least `desired_num`
        - stops containers until there are at most `desired_num` running
        - starts containers until there are at least `desired_num` running
        - removes all stopped containers
        """
        if not self.can_be_scaled():
            raise CannotBeScaledError()

        # Create enough containers
        containers = self.containers(stopped=True)
        while len(containers) < desired_num:
            log.info("Creating %s..." % self._next_container_name(containers))
            containers.append(self.create_container(detach=True))

        running_containers = []
        stopped_containers = []
        for c in containers:
            if c.is_running:
                running_containers.append(c)
            else:
                stopped_containers.append(c)
        running_containers.sort(key=lambda c: c.number)
        stopped_containers.sort(key=lambda c: c.number)

        # Stop containers
        while len(running_containers) > desired_num:
            c = running_containers.pop()
            log.info("Stopping %s..." % c.name)
            c.stop(timeout=1)
            stopped_containers.append(c)

        # Start containers
        while len(running_containers) < desired_num:
            c = stopped_containers.pop(0)
            log.info("Starting %s..." % c.name)
            self.start_container(c)
            running_containers.append(c)

        self.remove_stopped()

    def remove_stopped(self, **options):
        for c in self.containers(stopped=True):
            if not c.is_running:
                log.info("Removing %s..." % c.name)
                c.remove(**options)

    def create_container(self,
                         one_off=False,
                         insecure_registry=False,
                         do_build=True,
                         intermediate_container=None,
                         **override_options):
        """
        Create a container for this service. If the image doesn't exist, attempt to pull
        it.
        """
        container_options = self._get_container_create_options(
            override_options,
            one_off=one_off,
            intermediate_container=intermediate_container,
        )

        if (do_build and
                self.can_be_built() and
                not self.client.images(name=self.full_name)):
            self.build()

        try:
            return Container.create(self.client, **container_options)
        except APIError as e:
            if e.response.status_code == 404 and e.explanation and 'No such image' in str(e.explanation):
                log.info('Pulling image %s...' % container_options['image'])
                output = self.client.pull(
                    container_options['image'],
                    stream=True,
                    insecure_registry=insecure_registry
                )
                stream_output(output, sys.stdout)
                return Container.create(self.client, **container_options)
            raise

    def recreate_containers(self, insecure_registry=False, do_build=True, **override_options):
        """
        If a container for this service doesn't exist, create and start one. If there are
        any, stop them, create+start new ones, and remove the old containers.
        """
        containers = self.containers(stopped=True)
        if not containers:
            log.info("Creating %s..." % self._next_container_name(containers))
            container = self.create_container(
                insecure_registry=insecure_registry,
                do_build=do_build,
                **override_options)
            self.start_container(container)
            return [(None, container)]
        else:
            tuples = []

            for c in containers:
                log.info("Recreating %s..." % c.name)
                tuples.append(self.recreate_container(c, insecure_registry=insecure_registry, **override_options))

            return tuples

    def recreate_container(self, container, **override_options):
        """Recreate a container. An intermediate container is created so that
        the new container has the same name, while still supporting
        `volumes-from` the original container.
        """
        try:
            container.stop()
        except APIError as e:
            if (e.response.status_code == 500
                    and e.explanation
                    and 'no such process' in str(e.explanation)):
                pass
            else:
                raise

        intermediate_container = Container.create(
            self.client,
            image=container.image,
            entrypoint=['/bin/echo'],
            command=[],
            detach=True,
            host_config=create_host_config(volumes_from=[container.id]),
        )
        intermediate_container.start()
        intermediate_container.wait()
        container.remove()

        options = dict(override_options)
        new_container = self.create_container(
            do_build=False,
            intermediate_container=intermediate_container,
            **options
        )
        self.start_container(new_container)

        intermediate_container.remove()

        return (intermediate_container, new_container)

    def start_container_if_stopped(self, container):
        if container.is_running:
            return container
        else:
            log.info("Starting %s..." % container.name)
            return self.start_container(container)

    def start_container(self, container):
        container.start()
        return container

    def start_or_create_containers(
            self,
            insecure_registry=False,
            detach=False,
            do_build=True):
        containers = self.containers(stopped=True)

        if not containers:
            log.info("Creating %s..." % self._next_container_name(containers))
            new_container = self.create_container(
                insecure_registry=insecure_registry,
                detach=detach,
                do_build=do_build,
            )
            return [self.start_container(new_container)]
        else:
            return [self.start_container_if_stopped(c) for c in containers]

    def get_linked_names(self):
        return [s.name for (s, _) in self.links]

    def get_volumes_from_names(self):
        return [s.name for s in self.volumes_from if isinstance(s, Service)]

    def get_net_name(self):
        if isinstance(self.net, Service):
            return self.net.name
        else:
            return

    def _next_container_name(self, all_containers, one_off=False):
        bits = [self.project, self.name]
        if one_off:
            bits.append('run')
        return '_'.join(bits + [str(self._next_container_number(all_containers))])

    def _next_container_number(self, all_containers):
        numbers = [parse_name(c.name).number for c in all_containers]
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

    def _get_volumes_from(self, intermediate_container=None):
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

        if intermediate_container:
            volumes_from.append(intermediate_container.id)

        return volumes_from

    def _get_net(self):
        if not self.net:
            return "bridge"

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

    def _get_container_create_options(self, override_options, one_off=False, intermediate_container=None):
        container_options = dict(
            (k, self.options[k])
            for k in DOCKER_CONFIG_KEYS if k in self.options)
        container_options.update(override_options)

        container_options['name'] = self._next_container_name(
            self.containers(stopped=True, one_off=one_off),
            one_off)

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

        if 'volumes' in container_options:
            container_options['volumes'] = dict(
                (parse_volume_spec(v).internal, {})
                for v in container_options['volumes'])

        if self.can_be_built():
            container_options['image'] = self.full_name
        else:
            container_options['image'] = self._get_image_name(container_options['image'])

        # Delete options which are only used when starting
        for key in DOCKER_START_KEYS:
            container_options.pop(key, None)

        container_options['host_config'] = self._get_container_host_config(override_options, one_off=one_off, intermediate_container=intermediate_container)

        return container_options

    def _get_container_host_config(self, override_options, one_off=False, intermediate_container=None):
        options = dict(self.options, **override_options)
        port_bindings = build_port_bindings(options.get('ports') or [])

        volume_bindings = dict(
            build_volume_binding(parse_volume_spec(volume))
            for volume in options.get('volumes') or []
            if ':' in volume)

        privileged = options.get('privileged', False)
        cap_add = options.get('cap_add', None)
        cap_drop = options.get('cap_drop', None)
        pid = options.get('pid', None)

        dns = options.get('dns', None)
        if isinstance(dns, six.string_types):
            dns = [dns]

        dns_search = options.get('dns_search', None)
        if isinstance(dns_search, six.string_types):
            dns_search = [dns_search]

        restart = parse_restart_spec(options.get('restart', None))

        extra_hosts = build_extra_hosts(options.get('extra_hosts', None))

        return create_host_config(
            links=self._get_links(link_to_self=one_off),
            port_bindings=port_bindings,
            binds=volume_bindings,
            volumes_from=self._get_volumes_from(intermediate_container),
            privileged=privileged,
            network_mode=self._get_net(),
            dns=dns,
            dns_search=dns_search,
            restart_policy=restart,
            cap_add=cap_add,
            cap_drop=cap_drop,
            extra_hosts=extra_hosts,
            pid_mode=pid
        )

    def _get_image_name(self, image):
        repo, tag = parse_repository_tag(image)
        if tag == "":
            tag = "latest"
        return '%s:%s' % (repo, tag)

    def build(self, no_cache=False):
        log.info('Building %s...' % self.name)

        build_output = self.client.build(
            self.options['build'],
            tag=self.full_name,
            stream=True,
            rm=True,
            nocache=no_cache,
            dockerfile=self.options.get('dockerfile', None),
        )

        try:
            all_events = stream_output(build_output, sys.stdout)
        except StreamOutputError as e:
            raise BuildError(self, unicode(e))

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

    @property
    def full_name(self):
        """
        The tag to give to images built for this service.
        """
        return '%s_%s' % (self.project, self.name)

    def can_be_scaled(self):
        for port in self.options.get('ports', []):
            if ':' in str(port):
                return False
        return True

    def pull(self, insecure_registry=False):
        if 'image' in self.options:
            image_name = self._get_image_name(self.options['image'])
            log.info('Pulling %s (%s)...' % (self.name, image_name))
            self.client.pull(
                image_name,
                insecure_registry=insecure_registry
            )


NAME_RE = re.compile(r'^([^_]+)_([^_]+)_(run_)?(\d+)$')


def is_valid_name(name, one_off=False):
    match = NAME_RE.match(name)
    if match is None:
        return False
    if one_off:
        return match.group(3) == 'run_'
    else:
        return match.group(3) is None


def parse_name(name):
    match = NAME_RE.match(name)
    (project, service_name, _, suffix) = match.groups()
    return ServiceName(project, service_name, int(suffix))


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


def parse_volume_spec(volume_config):
    parts = volume_config.split(':')
    if len(parts) > 3:
        raise ConfigError("Volume %s has incorrect format, should be "
                          "external:internal[:mode]" % volume_config)

    if len(parts) == 1:
        return VolumeSpec(None, parts[0], 'rw')

    if len(parts) == 2:
        parts.append('rw')

    external, internal, mode = parts
    if mode not in ('rw', 'ro'):
        raise ConfigError("Volume %s has invalid mode (%s), should be "
                          "one of: rw, ro." % (volume_config, mode))

    return VolumeSpec(external, internal, mode)


def parse_repository_tag(s):
    if ":" not in s:
        return s, ""
    repo, tag = s.rsplit(":", 1)
    if "/" in tag:
        return s, ""
    return repo, tag


def build_volume_binding(volume_spec):
    internal = {'bind': volume_spec.internal, 'ro': volume_spec.mode == 'ro'}
    return volume_spec.external, internal


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
