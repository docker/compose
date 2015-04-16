from __future__ import unicode_literals
from __future__ import absolute_import
import logging

from functools import reduce
from .config import get_service_name_from_net, ConfigurationError
from .service import Service
from .container import Container
from docker.errors import APIError

log = logging.getLogger(__name__)


def sort_service_dicts(services):
    # Topological sort (Cormen/Tarjan algorithm).
    unmarked = services[:]
    temporary_marked = set()
    sorted_services = []

    def get_service_names(links):
        return [link.split(':')[0] for link in links]

    def get_service_dependents(service_dict, services):
        name = service_dict['name']
        return [
            service for service in services
            if (name in get_service_names(service.get('links', [])) or
                name in service.get('volumes_from', []) or
                name == get_service_name_from_net(service.get('net')))
        ]

    def visit(n):
        if n['name'] in temporary_marked:
            if n['name'] in get_service_names(n.get('links', [])):
                raise DependencyError('A service can not link to itself: %s' % n['name'])
            if n['name'] in n.get('volumes_from', []):
                raise DependencyError('A service can not mount itself as volume: %s' % n['name'])
            else:
                raise DependencyError('Circular import between %s' % ' and '.join(temporary_marked))
        if n in unmarked:
            temporary_marked.add(n['name'])
            for m in get_service_dependents(n, services):
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
            links = project.get_links(service_dict)
            volumes_from = project.get_volumes_from(service_dict)
            net = project.get_net(service_dict)

            project.services.append(Service(client=client, project=name, links=links, net=net,
                                            volumes_from=volumes_from, **service_dict))
        return project

    def get_service(self, name):
        """
        Retrieve a service by name. Raises NoSuchService
        if the named service does not exist.
        """
        for service in self.services:
            if service.name == name:
                return service

        raise NoSuchService(name)

    def get_services(self, service_names=None, include_deps=False):
        """
        Returns a list of this project's services filtered
        by the provided list of names, or all services if service_names is None
        or [].

        If include_deps is specified, returns a list including the dependencies for
        service_names, in order of dependency.

        Preserves the original order of self.services where possible,
        reordering as needed to resolve dependencies.

        Raises NoSuchService if any of the named services do not exist.
        """
        if service_names is None or len(service_names) == 0:
            return self.get_services(
                service_names=[s.name for s in self.services],
                include_deps=include_deps
            )
        else:
            unsorted = [self.get_service(name) for name in service_names]
            services = [s for s in self.services if s in unsorted]

            if include_deps:
                services = reduce(self._inject_deps, services, [])

            uniques = []
            [uniques.append(s) for s in services if s not in uniques]
            return uniques

    def get_links(self, service_dict):
        links = []
        if 'links' in service_dict:
            for link in service_dict.get('links', []):
                if ':' in link:
                    service_name, link_name = link.split(':', 1)
                else:
                    service_name, link_name = link, None
                try:
                    links.append((self.get_service(service_name), link_name))
                except NoSuchService:
                    raise ConfigurationError('Service "%s" has a link to service "%s" which does not exist.' % (service_dict['name'], service_name))
            del service_dict['links']
        return links

    def get_volumes_from(self, service_dict):
        volumes_from = []
        if 'volumes_from' in service_dict:
            for volume_name in service_dict.get('volumes_from', []):
                try:
                    service = self.get_service(volume_name)
                    volumes_from.append(service)
                except NoSuchService:
                    try:
                        container = Container.from_id(self.client, volume_name)
                        volumes_from.append(container)
                    except APIError:
                        raise ConfigurationError('Service "%s" mounts volumes from "%s", which is not the name of a service or container.' % (service_dict['name'], volume_name))
            del service_dict['volumes_from']
        return volumes_from

    def get_net(self, service_dict):
        if 'net' in service_dict:
            net_name = get_service_name_from_net(service_dict.get('net'))

            if net_name:
                try:
                    net = self.get_service(net_name)
                except NoSuchService:
                    try:
                        net = Container.from_id(self.client, net_name)
                    except APIError:
                        raise ConfigurationError('Serivce "%s" is trying to use the network of "%s", which is not the name of a service or container.' % (service_dict['name'], net_name))
            else:
                net = service_dict['net']

            del service_dict['net']

        else:
            net = 'bridge'

        return net

    def start(self, service_names=None, **options):
        for service in self.get_services(service_names):
            service.start(**options)

    def stop(self, service_names=None, **options):
        for service in reversed(self.get_services(service_names)):
            service.stop(**options)

    def kill(self, service_names=None, **options):
        for service in reversed(self.get_services(service_names)):
            service.kill(**options)

    def restart(self, service_names=None, **options):
        for service in self.get_services(service_names):
            service.restart(**options)

    def build(self, service_names=None, no_cache=False):
        for service in self.get_services(service_names):
            if service.can_be_built():
                service.build(no_cache)
            else:
                log.info('%s uses an image, skipping' % service.name)

    def up(self,
           service_names=None,
           start_deps=True,
           recreate=True,
           insecure_registry=False,
           detach=False,
           do_build=True):
        running_containers = []
        for service in self.get_services(service_names, include_deps=start_deps):
            if recreate:
                for (_, container) in service.recreate_containers(
                        insecure_registry=insecure_registry,
                        detach=detach,
                        do_build=do_build):
                    running_containers.append(container)
            else:
                for container in service.start_or_create_containers(
                        insecure_registry=insecure_registry,
                        detach=detach,
                        do_build=do_build):
                    running_containers.append(container)

        return running_containers

    def pull(self, service_names=None, insecure_registry=False):
        for service in self.get_services(service_names, include_deps=True):
            service.pull(insecure_registry=insecure_registry)

    def remove_stopped(self, service_names=None, **options):
        for service in self.get_services(service_names):
            service.remove_stopped(**options)

    def containers(self, service_names=None, stopped=False, one_off=False):
        return [Container.from_ps(self.client, container)
                for container in self.client.containers(all=stopped)
                for service in self.get_services(service_names)
                if service.has_container(container, one_off=one_off)]

    def _inject_deps(self, acc, service):
        net_name = service.get_net_name()
        dep_names = (service.get_linked_names() +
                     service.get_volumes_from_names() +
                     ([net_name] if net_name else []))

        if len(dep_names) > 0:
            dep_services = self.get_services(
                service_names=list(set(dep_names)),
                include_deps=True
            )
        else:
            dep_services = []

        dep_services.append(service)
        return acc + dep_services


class NoSuchService(Exception):
    def __init__(self, name):
        self.name = name
        self.msg = "No such service: %s" % self.name

    def __str__(self):
        return self.msg


class DependencyError(ConfigurationError):
    pass
