from __future__ import unicode_literals
from __future__ import absolute_import
from itertools import chain
import logging
from operator import (
    attrgetter,
    itemgetter,
)

from docker.errors import APIError
import six

from fig import includes
from fig.service import (
    Service,
    ServiceLink,
)
from fig.container import Container

log = logging.getLogger(__name__)


def sort_service_dicts(services):
    # Topological sort (Cormen/Tarjan algorithm).
    unmarked = sorted(services, key=itemgetter('name'))
    temporary_marked = set()
    sorted_services = []

    get_service_names = lambda links: [link.split(':')[0] for link in links]

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
            dependents = [m for m in services if (n['name'] in get_service_names(m.get('links', []))) or (n['name'] in m.get('volumes_from', []))]
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
    def __init__(self, name, services, client, namespace=None, external_projects=None):
        self.name = name
        self.services = services
        self.client = client
        # The top level project name is the namespace for included projects
        self.namespace = namespace or name
        self.external_projects = external_projects or []

    @classmethod
    def from_dicts(cls, name, service_dicts, client, namespace, external_projects):
        """
        Construct a ServiceCollection from a list of dicts representing services.
        """
        project = cls(name, [], client, namespace, external_projects)
        for service_dict in sort_service_dicts(service_dicts):
            links = project.get_links(service_dict.pop('links', None),
                                      service_dict['name'])
            volumes_from = project.get_volumes_from(service_dict)

            project.services.append(
                Service(client=client,
                        project=name,
                        links=links,
                        volumes_from=volumes_from,
                        **service_dict))
        return project

    @classmethod
    def from_config(cls, name, config, client, namespace=None, project_cache=None):
        services = []
        project_config = config.pop('project-config', {})
        external_projects = get_external_projects(
            project_config.pop('include', {}),
            project_config.pop('cache', {}),
            client,
            name,
            project_cache)

        for service_name, service in list(config.items()):
            if not isinstance(service, dict):
                raise ConfigurationError(
                    'Service "%s" doesn\'t have any configuration options. '
                    'All top level keys in your fig.yml must map to a '
                    'dictionary of configuration options.' % service_name)
            service['name'] = service_name
            services.append(service)
        return cls.from_dicts(name, services, client, namespace, external_projects)

    def get_service(self, name):
        """Retrieve a service by name.

        :param name: name of the service
        :returns: :class:`fig.service.Service`
        :raises NoSuchService: if no service was found by that name
        """
        if '_' in name:
            project_name, service_name = name.rsplit('_', 1)
            if project_name != self.namespace:
                # References (link, etc) do not contain the namespace, so add it
                project_name = self.namespace + project_name
        else:
            project_name, service_name = self.name, name

        if project_name == self.name:
            for service in self.services:
                if service.name == service_name:
                    return service

        for project in self.external_projects:
            if project.name == project_name:
                return project.get_service(service_name)

        raise NoSuchService(name)

    @property
    def all_services(self):
        return (flat_map(attrgetter('services'), self.external_projects) +
                self.services)

    def get_services(self, service_names=None, include_links=False):
        """
        Returns a list of this project's services filtered
        by the provided list of names, or all services if service_names is None
        or [].

        If include_links is specified, returns a list including the links for
        service_names, in order of dependency.

        Preserves the original order of self.services where possible,
        reordering as needed to resolve links.

        Raises NoSuchService if any of the named services do not exist.
        """

        def _add_linked_services(service):
            linked_services = service.get_linked_services()
            if not linked_services:
                return [service]

            return flat_map(_add_linked_services, linked_services) + [service]

        if not service_names:
            return self.all_services

        services = [self.get_service(name) for name in service_names]
        if include_links:
            services = flat_map(_add_linked_services, services)

        # TODO: use orderedset/ordereddict
        uniques = []
        [uniques.append(s) for s in services if s not in uniques]
        return uniques

    def get_links(self, config_links, name):
        def get_linked_service(link):
            if ':' in link:
                service_name, link_name = link.split(':', 1)
            else:
                service_name, link_name = link, None

            try:
                return ServiceLink(self.get_service(service_name), link_name)
            except NoSuchService:
                raise ConfigurationError(
                    'Service "%s" has a link to service "%s" which does not '
                    'exist.' % (name, service_name))

        return map(get_linked_service, config_links or [])

    def get_volumes_from(self, service_dict):
        volumes_from = []
        for volume_name in service_dict.pop('volumes_from', []):
            try:
                service = self.get_service(volume_name)
                volumes_from.append(service)
            except NoSuchService:
                try:
                    container = Container.from_id(self.client, volume_name)
                    volumes_from.append(container)
                except APIError:
                    raise ConfigurationError(
                        'Service "%s" mounts volumes from "%s", which is not '
                        'the name of a service or container.' % (
                            service_dict['name'], volume_name))
        return volumes_from

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
           start_links=True,
           recreate=True,
           insecure_registry=False,
           detach=False,
           do_build=True):
        running_containers = []
        for service in self.get_services(service_names, include_links=start_links):
            create_func = (service.recreate_containers if recreate
                           else service.start_or_create_containers)

            for container in create_func(
                    insecure_registry=insecure_registry,
                    detach=detach,
                    do_build=do_build):
                running_containers.append(container)

        return running_containers

    def pull(self, service_names=None, insecure_registry=False):
        for service in self.get_services(service_names, include_links=True):
            service.pull(insecure_registry=insecure_registry)

    def remove_stopped(self, service_names=None, **options):
        for service in self.get_services(service_names):
            service.remove_stopped(**options)

    def containers(self, service_names=None, stopped=False, one_off=False):
        return [Container.from_ps(self.client, container)
                for container in self.client.containers(all=stopped)
                for service in self.get_services(service_names)
                if service.has_container(container, one_off=one_off)]

    def __repr__(self):
        return "Project(%s, services=%s, includes=%s)" % (
            self.name,
            len(self.services),
            len(self.external_projects))


def flat_map(func, seq):
    return list(chain.from_iterable(map(func, seq)))


def get_external_projects(
        includes_config,
        cache_config,
        client,
        project_name,
        project_cache):
    """Recursively fetch included projects.

    Cache each external project by url. If a project is encountered with the
    same url the same instance of :class:`Project` will be returned.
    """
    def build_project(name, *args, **kwargs):
        name = '%s%s' % (project_name, name)
        kwargs['namespace'] = project_name
        return Project.from_config(name, *args, **kwargs)

    if project_cache is None:
        project_cache = includes.ExternalProjectCache(
            includes.LocalConfigCache.from_config(cache_config),
            client,
            build_project)

    return [project_cache.get_project_from_include(*item)
            for item in six.iteritems(includes_config)]


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
