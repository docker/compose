from __future__ import absolute_import
from __future__ import unicode_literals

import logging
from functools import reduce

from docker.errors import APIError
from docker.errors import NotFound

from . import parallel
from .config import ConfigurationError
from .config.sort_services import get_service_name_from_net
from .const import DEFAULT_TIMEOUT
from .const import LABEL_ONE_OFF
from .const import LABEL_PROJECT
from .const import LABEL_SERVICE
from .container import Container
from .service import ContainerNet
from .service import ConvergenceStrategy
from .service import Net
from .service import Service
from .service import ServiceNet


log = logging.getLogger(__name__)


class Project(object):
    """
    A collection of services.
    """
    def __init__(self, name, services, client, use_networking=False, network_driver=None):
        self.name = name
        self.services = services
        self.client = client
        self.use_networking = use_networking
        self.network_driver = network_driver

    def labels(self, one_off=False):
        return [
            '{0}={1}'.format(LABEL_PROJECT, self.name),
            '{0}={1}'.format(LABEL_ONE_OFF, "True" if one_off else "False"),
        ]

    @classmethod
    def from_dicts(cls, name, service_dicts, client, use_networking=False, network_driver=None):
        """
        Construct a ServiceCollection from a list of dicts representing services.
        """
        project = cls(name, [], client, use_networking=use_networking, network_driver=network_driver)

        if use_networking:
            remove_links(service_dicts)

        for service_dict in service_dicts:
            links = project.get_links(service_dict)
            volumes_from = project.get_volumes_from(service_dict)
            net = project.get_net(service_dict)

            project.services.append(
                Service(
                    client=client,
                    project=name,
                    use_networking=use_networking,
                    links=links,
                    net=net,
                    volumes_from=volumes_from,
                    **service_dict))
        return project

    @property
    def service_names(self):
        return [service.name for service in self.services]

    def get_service(self, name):
        """
        Retrieve a service by name. Raises NoSuchService
        if the named service does not exist.
        """
        for service in self.services:
            if service.name == name:
                return service

        raise NoSuchService(name)

    def validate_service_names(self, service_names):
        """
        Validate that the given list of service names only contains valid
        services. Raises NoSuchService if one of the names is invalid.
        """
        valid_names = self.service_names
        for name in service_names:
            if name not in valid_names:
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
                service_names=self.service_names,
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
                    raise ConfigurationError(
                        'Service "%s" has a link to service "%s" which does not '
                        'exist.' % (service_dict['name'], service_name))
            del service_dict['links']
        return links

    def get_volumes_from(self, service_dict):
        volumes_from = []
        if 'volumes_from' in service_dict:
            for volume_from_spec in service_dict.get('volumes_from', []):
                # Get service
                try:
                    service = self.get_service(volume_from_spec.source)
                    volume_from_spec = volume_from_spec._replace(source=service)
                except NoSuchService:
                    try:
                        container = Container.from_id(self.client, volume_from_spec.source)
                        volume_from_spec = volume_from_spec._replace(source=container)
                    except APIError:
                        raise ConfigurationError(
                            'Service "%s" mounts volumes from "%s", which is '
                            'not the name of a service or container.' % (
                                service_dict['name'],
                                volume_from_spec.source))
                volumes_from.append(volume_from_spec)
            del service_dict['volumes_from']
        return volumes_from

    def get_net(self, service_dict):
        net = service_dict.pop('net', None)
        if not net:
            if self.use_networking:
                return Net(self.name)
            return Net(None)

        net_name = get_service_name_from_net(net)
        if not net_name:
            return Net(net)

        try:
            return ServiceNet(self.get_service(net_name))
        except NoSuchService:
            pass
        try:
            return ContainerNet(Container.from_id(self.client, net_name))
        except APIError:
            raise ConfigurationError(
                'Service "%s" is trying to use the network of "%s", '
                'which is not the name of a service or container.' % (
                    service_dict['name'],
                    net_name))

    def start(self, service_names=None, **options):
        for service in self.get_services(service_names):
            service.start(**options)

    def stop(self, service_names=None, **options):
        parallel.parallel_stop(self.containers(service_names), options)

    def pause(self, service_names=None, **options):
        parallel.parallel_pause(reversed(self.containers(service_names)), options)

    def unpause(self, service_names=None, **options):
        parallel.parallel_unpause(self.containers(service_names), options)

    def kill(self, service_names=None, **options):
        parallel.parallel_kill(self.containers(service_names), options)

    def remove_stopped(self, service_names=None, **options):
        parallel.parallel_remove(self.containers(service_names, stopped=True), options)

    def restart(self, service_names=None, **options):
        parallel.parallel_restart(self.containers(service_names, stopped=True), options)

    def build(self, service_names=None, no_cache=False, pull=False, force_rm=False):
        for service in self.get_services(service_names):
            if service.can_be_built():
                service.build(no_cache, pull, force_rm)
            else:
                log.info('%s uses an image, skipping' % service.name)

    def up(self,
           service_names=None,
           start_deps=True,
           strategy=ConvergenceStrategy.changed,
           do_build=True,
           timeout=DEFAULT_TIMEOUT,
           detached=False):

        services = self.get_services(service_names, include_deps=start_deps)

        for service in services:
            service.remove_duplicate_containers()

        plans = self._get_convergence_plans(services, strategy)

        if self.use_networking and self.uses_default_network():
            self.ensure_network_exists()

        return [
            container
            for service in services
            for container in service.execute_convergence_plan(
                plans[service.name],
                do_build=do_build,
                timeout=timeout,
                detached=detached
            )
        ]

    def _get_convergence_plans(self, services, strategy):
        plans = {}

        for service in services:
            updated_dependencies = [
                name
                for name in service.get_dependency_names()
                if name in plans
                and plans[name].action in ('recreate', 'create')
            ]

            if updated_dependencies and strategy.allows_recreate:
                log.debug('%s has upstream changes (%s)',
                          service.name,
                          ", ".join(updated_dependencies))
                plan = service.convergence_plan(ConvergenceStrategy.always)
            else:
                plan = service.convergence_plan(strategy)

            plans[service.name] = plan

        return plans

    def pull(self, service_names=None, ignore_pull_failures=False):
        for service in self.get_services(service_names, include_deps=False):
            service.pull(ignore_pull_failures)

    def containers(self, service_names=None, stopped=False, one_off=False):
        if service_names:
            self.validate_service_names(service_names)
        else:
            service_names = self.service_names

        containers = list(filter(None, [
            Container.from_ps(self.client, container)
            for container in self.client.containers(
                all=stopped,
                filters={'label': self.labels(one_off=one_off)})]))

        def matches_service_names(container):
            return container.labels.get(LABEL_SERVICE) in service_names

        return [c for c in containers if matches_service_names(c)]

    def get_network(self):
        try:
            return self.client.inspect_network(self.name)
        except NotFound:
            return None

    def ensure_network_exists(self):
        # TODO: recreate network if driver has changed?
        if self.get_network() is None:
            driver_name = 'the default driver'
            if self.network_driver:
                driver_name = 'driver "{}"'.format(self.network_driver)

            log.info(
                'Creating network "{}" with {}'
                .format(self.name, driver_name)
            )
            self.client.create_network(self.name, driver=self.network_driver)

    def remove_network(self):
        network = self.get_network()
        if network:
            self.client.remove_network(network['Id'])

    def uses_default_network(self):
        return any(service.net.mode == self.name for service in self.services)

    def _inject_deps(self, acc, service):
        dep_names = service.get_dependency_names()

        if len(dep_names) > 0:
            dep_services = self.get_services(
                service_names=list(set(dep_names)),
                include_deps=True
            )
        else:
            dep_services = []

        dep_services.append(service)
        return acc + dep_services


def remove_links(service_dicts):
    services_with_links = [s for s in service_dicts if 'links' in s]
    if not services_with_links:
        return

    if len(services_with_links) == 1:
        prefix = '"{}" defines'.format(services_with_links[0]['name'])
    else:
        prefix = 'Some services ({}) define'.format(
            ", ".join('"{}"'.format(s['name']) for s in services_with_links))

    log.warn(
        '\n{} links, which are not compatible with Docker networking and will be ignored.\n'
        'Future versions of Docker will not support links - you should remove them for '
        'forwards-compatibility.\n'.format(prefix))

    for s in services_with_links:
        del s['links']


class NoSuchService(Exception):
    def __init__(self, name):
        self.name = name
        self.msg = "No such service: %s" % self.name

    def __str__(self):
        return self.msg
