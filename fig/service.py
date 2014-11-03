from __future__ import unicode_literals
from __future__ import absolute_import
from collections import namedtuple
import logging
import re
import os
from operator import attrgetter
import sys

from docker.errors import APIError

from .container import Container
from .progress_stream import stream_output, StreamOutputError

log = logging.getLogger(__name__)


DOCKER_CONFIG_KEYS = ['image', 'command', 'hostname', 'domainname', 'user', 'detach', 'stdin_open', 'tty', 'mem_limit', 'ports', 'environment', 'dns', 'volumes', 'entrypoint', 'privileged', 'volumes_from', 'net', 'working_dir']
DOCKER_CONFIG_HINTS = {
    'link'      : 'links',
    'port'      : 'ports',
    'privilege' : 'privileged',
    'priviliged': 'privileged',
    'privilige' : 'privileged',
    'volume'    : 'volumes',
    'workdir'   : 'working_dir',
}

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
    def __init__(self, name, client=None, project='default', links=None, volumes_from=None, **options):
        if not re.match('^%s+$' % VALID_NAME_CHARS, name):
            raise ConfigError('Invalid service name "%s" - only %s are allowed' % (name, VALID_NAME_CHARS))
        if not re.match('^%s+$' % VALID_NAME_CHARS, project):
            raise ConfigError('Invalid project name "%s" - only %s are allowed' % (project, VALID_NAME_CHARS))
        if 'image' in options and 'build' in options:
            raise ConfigError('Service %s has both an image and build path specified. A service can either be built to image or use an existing image, not both.' % name)

        supported_options = DOCKER_CONFIG_KEYS + ['build', 'expose']

        for k in options:
            if k not in supported_options:
                msg = "Unsupported config option for %s service: '%s'" % (name, k)
                if k in DOCKER_CONFIG_HINTS:
                    msg += " (did you mean '%s'?)" % DOCKER_CONFIG_HINTS[k]
                raise ConfigError(msg)

        self.name = name
        self.client = client
        self.project = project
        self.links = links or []
        self.volumes_from = volumes_from or []
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
        """Return a :class:`fig.container.Container` for this service. The
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
        Adjusts the number of containers to the specified number and ensures they are running.

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
            containers.append(self.create_container())

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

    def create_container(self, one_off=False, insecure_registry=False, **override_options):
        """
        Create a container for this service. If the image doesn't exist, attempt to pull
        it.
        """
        container_options = self._get_container_create_options(override_options, one_off=one_off)
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

    def recreate_containers(self, insecure_registry=False, **override_options):
        """
        If a container for this service doesn't exist, create and start one. If there are
        any, stop them, create+start new ones, and remove the old containers.
        """
        containers = self.containers(stopped=True)
        if not containers:
            log.info("Creating %s..." % self._next_container_name(containers))
            container = self.create_container(insecure_registry=insecure_registry, **override_options)
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
        )
        intermediate_container.start(volumes_from=container.id)
        intermediate_container.wait()
        container.remove()

        options = dict(override_options)
        new_container = self.create_container(**options)
        self.start_container(new_container, intermediate_container=intermediate_container)

        intermediate_container.remove()

        return (intermediate_container, new_container)

    def start_container_if_stopped(self, container, **options):
        if container.is_running:
            return container
        else:
            log.info("Starting %s..." % container.name)
            return self.start_container(container, **options)

    def start_container(self, container=None, intermediate_container=None, **override_options):
        container = container or self.create_container(**override_options)
        options = dict(self.options, **override_options)
        ports = dict(split_port(port) for port in options.get('ports') or [])

        volume_bindings = dict(
            build_volume_binding(parse_volume_spec(volume))
            for volume in options.get('volumes') or []
            if ':' in volume)

        privileged = options.get('privileged', False)
        net = options.get('net', 'bridge')
        dns = options.get('dns', None)

        container.start(
            links=self._get_links(link_to_self=options.get('one_off', False)),
            port_bindings=ports,
            binds=volume_bindings,
            volumes_from=self._get_volumes_from(intermediate_container),
            privileged=privileged,
            network_mode=net,
            dns=dns,
        )
        return container

    def start_or_create_containers(self, insecure_registry=False):
        containers = self.containers(stopped=True)

        if not containers:
            log.info("Creating %s..." % self._next_container_name(containers))
            new_container = self.create_container(insecure_registry=insecure_registry)
            return [self.start_container(new_container)]
        else:
            return [self.start_container_if_stopped(c) for c in containers]

    def get_linked_names(self):
        return [s.name for (s, _) in self.links]

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

    def _get_container_create_options(self, override_options, one_off=False):
        container_options = dict((k, self.options[k]) for k in DOCKER_CONFIG_KEYS if k in self.options)
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

        if 'environment' in container_options:
            if isinstance(container_options['environment'], list):
                container_options['environment'] = dict(split_env(e) for e in container_options['environment'])
            container_options['environment'] = dict(resolve_env(k, v) for k, v in container_options['environment'].iteritems())

        if self.can_be_built():
            if len(self.client.images(name=self._build_tag_name())) == 0:
                self.build()
            container_options['image'] = self._build_tag_name()

        # Delete options which are only used when starting
        for key in ['privileged', 'net', 'dns']:
            if key in container_options:
                del container_options[key]

        return container_options

    def build(self, no_cache=False):
        log.info('Building %s...' % self.name)

        build_output = self.client.build(
            self.options['build'],
            tag=self._build_tag_name(),
            stream=True,
            rm=True,
            nocache=no_cache,
        )

        try:
            all_events = stream_output(build_output, sys.stdout)
        except StreamOutputError, e:
            raise BuildError(self, unicode(e))

        image_id = None

        for event in all_events:
            if 'stream' in event:
                match = re.search(r'Successfully built ([0-9a-f]+)', event.get('stream', ''))
                if match:
                    image_id = match.group(1)

        if image_id is None:
            raise BuildError(self)

        return image_id

    def can_be_built(self):
        return 'build' in self.options

    def _build_tag_name(self):
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
            log.info('Pulling %s (%s)...' % (self.name, self.options.get('image')))
            self.client.pull(
                self.options.get('image'),
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


def get_container_name(container):
    if not container.get('Name') and not container.get('Names'):
        return None
    # inspect
    if 'Name' in container:
        return container['Name']
    # ps
    for name in container['Names']:
        if len(name.split('/')) == 2:
            return name[1:]


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


def build_volume_binding(volume_spec):
    internal = {'bind': volume_spec.internal, 'ro': volume_spec.mode == 'ro'}
    external = os.path.expanduser(volume_spec.external)
    return os.path.abspath(os.path.expandvars(external)), internal


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
