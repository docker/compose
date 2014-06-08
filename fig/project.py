from __future__ import unicode_literals
from __future__ import absolute_import
import logging
from .service import Service

log = logging.getLogger(__name__)


def sort_service_dicts(services):
    # Topological sort (Cormen/Tarjan algorithm).
    unmarked = services[:]
    temporary_marked = set()
    sorted_services = []

    get_service_names = lambda links: [link.split(':')[0] for link in links]

    def visit(n):
        if n['name'] in temporary_marked:
            if n['name'] in get_service_names(n.get('links', [])):
                raise DependencyError('A service can not link to itself: %s' % n['name'])
            else:
                raise DependencyError('Circular import between %s' % ' and '.join(temporary_marked))
        if n in unmarked:
            temporary_marked.add(n['name'])
            dependents = [m for m in services if n['name'] in get_service_names(m.get('links', []))]
            for m in dependents:
                visit(m)
            temporary_marked.remove(n['name'])
            unmarked.remove(n)
            sorted_services.insert(0, n)

    while unmarked:
        visit(unmarked[-1])

    return sorted_services

class Project(object):
    """
    A collection of services.
    """
    def __init__(self, name, services, client):
        self.name = name
        self.services = services
        self.client = client

    @classmethod
    def from_dicts(cls, name, service_dicts, client):
        """
        Construct a ServiceCollection from a list of dicts representing services.
        """
        project = cls(name, [], client)
        for service_dict in sort_service_dicts(service_dicts):
            # Reference links by object
            links = []
            if 'links' in service_dict:
                for link in service_dict.get('links', []):
                    if ':' in link:
                        service_name, link_name = link.split(':', 1)
                    else:
                        service_name, link_name = link, None
                    try:
                        links.append((project.get_service(service_name), link_name))
                    except NoSuchService:
                        raise ConfigurationError('Service "%s" has a link to service "%s" which does not exist.' % (service_dict['name'], service_name))

                del service_dict['links']

            project.services.append(Service(client=client, project=name, links=links, **service_dict))
        return project

    @classmethod
    def from_config(cls, name, config, client):
        dicts = []
        for service_name, service in list(config.items()):
            if not isinstance(service, dict):
                raise ConfigurationError('Service "%s" doesn\'t have any configuration options. All top level keys in your fig.yml must map to a dictionary of configuration options.')
            service['name'] = service_name
            dicts.append(service)
        return cls.from_dicts(name, dicts, client)

    def get_service(self, name):
        """
        Retrieve a service by name. Raises NoSuchService
        if the named service does not exist.
        """
        for service in self.services:
            if service.name == name:
                return service

        raise NoSuchService(name)

    def get_services(self, service_names=None, include_links=False):
        """
        Returns a list of this project's services filtered
        by the provided list of names, or all auto_start services if
        service_names is None or [].

        If include_links is specified, returns a list including the links for
        service_names, in order of dependency.

        Preserves the original order of self.services where possible,
        reordering as needed to resolve links.

        Raises NoSuchService if any of the named services do not exist.
        """
        if service_names is None or len(service_names) == 0:
            return [s for s in self.services if s.options['auto_start']]
        else:
            unsorted = [self.get_service(name) for name in service_names]
            services = [s for s in self.services if s in unsorted]

            if include_links:
                services = reduce(self._prepend_with_links, services, [])

            uniques = []
            [uniques.append(s) for s in services if s not in uniques]
            return uniques

    def start(self, service_names=None, **options):
        for service in self.get_services(service_names):
            service.start(**options)

    def stop(self, service_names=None, **options):
        for service in reversed(self.get_services(service_names)):
            service.stop(**options)

    def kill(self, service_names=None, **options):
        for service in reversed(self.get_services(service_names)):
            service.kill(**options)

    def build(self, service_names=None, **options):
        for service in self.get_services(service_names):
            if service.can_be_built():
                service.build(**options)
            else:
                log.info('%s uses an image, skipping' % service.name)

    def up(self, service_names=None, start_links=True):
        new_containers = []

        for service in self.get_services(service_names, include_links=start_links):
            for (_, new) in service.recreate_containers():
                new_containers.append(new)

        return new_containers

    def remove_stopped(self, service_names=None, **options):
        for service in self.get_services(service_names):
            service.remove_stopped(**options)

    def containers(self, service_names=None, *args, **kwargs):
        l = []
        for service in self.get_services(service_names):
            for container in service.containers(*args, **kwargs):
                l.append(container)
        return l

    def _prepend_with_links(self, acc, service):
        linked_services = self.get_services(
            service_names=service.get_linked_names(),
            include_links=True
        )
        linked_services.append(service)
        return acc + linked_services


class NoSuchService(Exception):
    def __init__(self, name):
        self.name = name
        self.msg = "No such service: %s" % self.name

    def __str__(self):
        return self.msg


class ConfigurationError(Exception):
    def __init__(self, msg):
        self.msg = msg

    def __str__(self):
        return self.msg

class DependencyError(ConfigurationError):
    pass

