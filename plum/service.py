import logging
import re

log = logging.getLogger(__name__)

class Service(object):
    def __init__(self, name, client=None, links=[], **options):
        if not re.match('^[a-zA-Z0-9_]+$', name):
            raise ValueError('Invalid name: %s' % name)
        if 'image' in options and 'build' in options:
            raise ValueError('Service %s has both an image and build path specified. A service can either be built to image or use an existing image, not both.')

        self.name = name
        self.client = client
        self.links = links or []
        self.options = options

    @property
    def containers(self):
        return [c for c in self.client.containers(all=True) if parse_name(get_container_name(c))[0] == self.name]

    def start(self):
        if len(self.containers) == 0:
            return self.start_container()

    def stop(self):
        self.scale(0)

    def scale(self, num):
        while len(self.containers) < num:
            self.start_container()

        while len(self.containers) > num:
            self.stop_container()

    def create_container(self, **override_options):
        container_options = self._get_container_options(override_options)
        return self.client.create_container(**container_options)

    def start_container(self, container=None, **override_options):
        container_options = self._get_container_options(override_options)
        if container is None:
            container = self.create_container(**override_options)
        port_bindings = {}
        for port in self.options.get('ports', []):
            port = unicode(port)
            if ':' in port:
                internal_port, external_port = port.split(':', 1)
                port_bindings[int(internal_port)] = int(external_port)
            else:
                port_bindings[int(port)] = None
        self.client.start(
            container['Id'],
            links=self._get_links(),
            port_bindings=port_bindings,
        )
        return container

    def stop_container(self):
        container_id = self.containers[-1]['Id']
        self.client.kill(container_id)
        self.client.remove_container(container_id)

    def next_container_number(self):
        numbers = [parse_name(get_container_name(c))[1] for c in self.containers]

        if len(numbers) == 0:
            return 1
        else:
            return max(numbers) + 1

    def get_names(self):
        return [get_container_name(c) for c in self.containers]

    def inspect(self):
        return [self.client.inspect_container(c['Id']) for c in self.containers]

    def _get_links(self):
        links = {}
        for service in self.links:
            for name in service.get_names():
                links[name] = name
        return links

    def _get_container_options(self, override_options):
        keys = ['image', 'command', 'hostname', 'user', 'detach', 'stdin_open', 'tty', 'mem_limit', 'ports', 'environment', 'dns', 'volumes', 'volumes_from']
        container_options = dict((k, self.options[k]) for k in keys if k in self.options)
        container_options.update(override_options)

        number = self.next_container_number()
        container_options['name'] = make_name(self.name, number)

        if 'ports' in container_options:
            container_options['ports'] = [unicode(p).split(':')[0] for p in container_options['ports']]

        if 'build' in self.options:
            log.info('Building %s from %s...' % (self.name, self.options['build']))
            container_options['image'] = self.client.build(self.options['build'])[0]

        return container_options


def make_name(prefix, number):
    return '%s_%s' % (prefix, number)


def parse_name(name):
    match = re.match('^(.+)_(\d+)$', name)

    if match is None:
        raise ValueError("Invalid name: %s" % name)

    (service_name, suffix) = match.groups()

    return (service_name, int(suffix))


def get_container_name(container):
    for name in container['Names']:
        if len(name.split('/')) == 2:
            return name[1:]
