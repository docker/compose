from __future__ import unicode_literals
from __future__ import absolute_import
from .packages.docker.client import APIError
import logging
import re
import os
import sys
from .container import Container

log = logging.getLogger(__name__)


class BuildError(Exception):
    pass


class CannotBeScaledError(Exception):
    pass


class Service(object):
    def __init__(self, name, client=None, project='default', links=[], **options):
        if not re.match('^[a-zA-Z0-9]+$', name):
            raise ValueError('Invalid name: %s' % name)
        if not re.match('^[a-zA-Z0-9]+$', project):
            raise ValueError('Invalid project: %s' % project)
        if 'image' in options and 'build' in options:
            raise ValueError('Service %s has both an image and build path specified. A service can either be built to image or use an existing image, not both.' % name)

        self.name = name
        self.client = client
        self.project = project
        self.links = links or []
        self.options = options

    def containers(self, stopped=False, one_off=False):
        l = []
        for container in self.client.containers(all=stopped):
            name = get_container_name(container)
            if not name or not is_valid_name(name, one_off):
                continue
            project, name, number = parse_name(name)
            if project == self.project and name == self.name:
                l.append(Container.from_ps(self.client, container))
        return l

    def start(self, **options):
        for c in self.containers(stopped=True):
            if not c.is_running:
                log.info("Starting %s..." % c.name)
                self.start_container(c, **options)

    def stop(self, **options):
        for c in self.containers():
            log.info("Stopping %s..." % c.name)
            c.stop(**options)

    def kill(self, **options):
        for c in self.containers():
            log.info("Killing %s..." % c.name)
            c.kill(**options)

    def scale(self, desired_num):
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
            c.start()
            running_containers.append(c)


    def remove_stopped(self, **options):
        for c in self.containers(stopped=True):
            if not c.is_running:
                log.info("Removing %s..." % c.name)
                c.remove(**options)

    def create_container(self, one_off=False, **override_options):
        """
        Create a container for this service. If the image doesn't exist, attempt to pull
        it.
        """
        container_options = self._get_container_options(override_options, one_off=one_off)
        try:
            return Container.create(self.client, **container_options)
        except APIError as e:
            if e.response.status_code == 404 and e.explanation and 'No such image' in str(e.explanation):
                log.info('Pulling image %s...' % container_options['image'])
                self.client.pull(container_options['image'])
                return Container.create(self.client, **container_options)
            raise

    def recreate_containers(self, **override_options):
        """
        If a container for this service doesn't exist, create one. If there are
        any, stop them and create new ones. Does not remove the old containers.
        """
        containers = self.containers(stopped=True)

        if len(containers) == 0:
            log.info("Creating %s..." % self.next_container_name())
            return ([], [self.create_container(**override_options)])
        else:
            old_containers = []
            new_containers = []

            for c in containers:
                log.info("Recreating %s..." % c.name)
                (old_container, new_container) = self.recreate_container(c, **override_options)
                old_containers.append(old_container)
                new_containers.append(new_container)

            return (old_containers, new_containers)

    def recreate_container(self, container, **override_options):
        if container.is_running:
            container.stop(timeout=1)

        intermediate_container = Container.create(
            self.client,
            image=container.image,
            command='echo',
            volumes_from=container.id,
            entrypoint=None
        )
        intermediate_container.start()
        intermediate_container.wait()
        container.remove()

        options = dict(override_options)
        options['volumes_from'] = intermediate_container.id
        new_container = self.create_container(**options)

        return (intermediate_container, new_container)

    def start_container(self, container=None, **override_options):
        if container is None:
            container = self.create_container(**override_options)

        options = self.options.copy()
        options.update(override_options)

        port_bindings = {}

        # FIXME : debug
        from pprint import pprint
        if options.get('ports', None) is not None:
                    # FIXME : check udp for this port too
            pprint(options['ports'])
        # ENDFIXME

            for port in options['ports']:
                port = str(port)
                if ':' in port:

                    # FIXME : check if . is in one of the port
                    # -> . == ip_adresse
                    # size is not a good indicator, you cand do:
                    # 4243
                    # 4243:4243
                    # 127.0.0.1:4243
                    # 127.0.0.1:4243:4243
                    external_port, internal_port = port.split(':', 1)

                    # FIXME : test
                    if external_port.find('.') != -1:
                        port_split = internal_port.split(':', 1)
                        if len(port_split) > 1:
                            external_port = (external_port, port_split[0])
                            internal_port = port_split[1]
                        else:
                            external_port = (external_port)
                            internal_port = port_split[0]
                        print external_port, internal_port
                else:
                    external_port, internal_port = (None, port)

                port_bindings[internal_port] = external_port

        # FIXME : debug
        pprint(port_bindings)
        # ENDFIXME

        volume_bindings = {}

        if options.get('volumes', None) is not None:
            for volume in options['volumes']:
                if ':' in volume:
                    external_dir, internal_dir = volume.split(':')
                    volume_bindings[os.path.abspath(external_dir)] = internal_dir

        container.start(
            links=self._get_links(),
            port_bindings=port_bindings,
            binds=volume_bindings,
        )
        return container

    def next_container_name(self, one_off=False):
        bits = [self.project, self.name]
        if one_off:
            bits.append('run')
        return '_'.join(bits + [str(self.next_container_number(one_off=one_off))])

    def next_container_number(self, one_off=False):
        numbers = [parse_name(c.name)[2] for c in self.containers(stopped=True, one_off=one_off)]

        if len(numbers) == 0:
            return 1
        else:
            return max(numbers) + 1

    def _get_links(self):
        links = []
        for service in self.links:
            for container in service.containers():
                links.append((container.name, container.name))
                links.append((container.name, container.name_without_project))
        for container in self.containers():
            links.append((container.name, container.name))
            links.append((container.name, container.name_without_project))
        return links

    def _get_container_options(self, override_options, one_off=False):
        keys = ['image', 'command', 'hostname', 'user', 'detach', 'stdin_open', 'tty', 'mem_limit', 'ports', 'environment', 'dns', 'volumes', 'volumes_from', 'entrypoint']
        container_options = dict((k, self.options[k]) for k in keys if k in self.options)
        container_options.update(override_options)

        container_options['name'] = self.next_container_name(one_off)

        if 'ports' in container_options:
            ports = []
            for port in container_options['ports']:
                port = str(port)
                if ':' in port:
                    port = port.split(':')[-1]
                if '/' in port:
                    port = tuple(port.split('/'))
                ports.append(port)
            container_options['ports'] = ports
            from pprint import pprint
            print "create:"
            pprint(container_options)

        if 'volumes' in container_options:
            container_options['volumes'] = dict((split_volume(v)[1], {}) for v in container_options['volumes'])

        if self.can_be_built():
            if len(self.client.images(name=self._build_tag_name())) == 0:
                self.build()
            container_options['image'] = self._build_tag_name()

        return container_options

    def build(self):
        log.info('Building %s...' % self.name)

        build_output = self.client.build(
            self.options['build'],
            tag=self._build_tag_name(),
            stream=True
        )

        image_id = None

        for line in build_output:
            if line:
                match = re.search(r'Successfully built ([0-9a-f]+)', line)
                if match:
                    image_id = match.group(1)
            sys.stdout.write(line)

        if image_id is None:
            raise BuildError()

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


NAME_RE = re.compile(r'^([^_]+)_([^_]+)_(run_)?(\d+)$')


def is_valid_name(name, one_off=False):
    match = NAME_RE.match(name)
    if match is None:
        return False
    if one_off:
        return match.group(3) == 'run_'
    else:
        return match.group(3) is None


def parse_name(name, one_off=False):
    match = NAME_RE.match(name)
    (project, service_name, _, suffix) = match.groups()
    return (project, service_name, int(suffix))


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


def split_volume(v):
    """
    If v is of the format EXTERNAL:INTERNAL, returns (EXTERNAL, INTERNAL).
    If v is of the format INTERNAL, returns (None, INTERNAL).
    """
    if ':' in v:
        return v.split(':', 1)
    else:
        return (None, v)
