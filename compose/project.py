from __future__ import absolute_import
from __future__ import unicode_literals

import datetime
import logging
from functools import reduce

from docker.errors import APIError
from docker.errors import NotFound

from . import parallel
from .config import ConfigurationError
from .config.sort_services import get_service_name_from_net
from .const import DEFAULT_TIMEOUT
from .const import IMAGE_EVENTS
from .const import LABEL_ONE_OFF
from .const import LABEL_PROJECT
from .const import LABEL_SERVICE
from .container import Container
from .network import Network
from .service import ContainerNet
from .service import ConvergenceStrategy
from .service import Net
from .service import Service
from .service import ServiceNet
from .utils import microseconds_from_time_nano
from .volume import Volume


log = logging.getLogger(__name__)


class Project(object):
    """
    A collection of services.
    """
    def __init__(self, name, services, client, networks=None, volumes=None,
                 use_networking=False, network_driver=None):
        self.name = name
        self.services = services
        self.client = client
        self.use_networking = use_networking
        self.network_driver = network_driver
        self.networks = networks or []
        self.volumes = volumes or []

    def labels(self, one_off=False):
        return [
            '{0}={1}'.format(LABEL_PROJECT, self.name),
            '{0}={1}'.format(LABEL_ONE_OFF, "True" if one_off else "False"),
        ]

    @classmethod
    def from_config(cls, name, config_data, client):
        """
        Construct a Project from a config.Config object.
        """
        use_networking = (config_data.version and config_data.version >= 2)
        project = cls(name, [], client, use_networking=use_networking)

        network_config = config_data.networks or {}
        custom_networks = [
            Network(
                client=client, project=name, name=network_name,
                driver=data.get('driver'),
                driver_opts=data.get('driver_opts'),
                external_name=data.get('external_name'),
            )
            for network_name, data in network_config.items()
        ]

        all_networks = custom_networks[:]
        if 'default' not in network_config:
            all_networks.append(project.default_network)

        for service_dict in config_data.services:
            if use_networking:
                networks = get_networks(service_dict, all_networks)
                net = Net(networks[0]) if networks else Net("none")
                links = []
            else:
                networks = []
                net = project.get_net(service_dict)
                links = project.get_links(service_dict)

            volumes_from = get_volumes_from(project, service_dict)

            project.services.append(
                Service(
                    client=client,
                    project=name,
                    use_networking=use_networking,
                    networks=networks,
                    links=links,
                    net=net,
                    volumes_from=volumes_from,
                    **service_dict))

        project.networks += custom_networks
        if 'default' not in network_config and project.uses_default_network():
            project.networks.append(project.default_network)

        if config_data.volumes:
            for vol_name, data in config_data.volumes.items():
                project.volumes.append(
                    Volume(
                        client=client, project=name, name=vol_name,
                        driver=data.get('driver'),
                        driver_opts=data.get('driver_opts'),
                        external_name=data.get('external_name')
                    )
                )

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
            service_names = self.service_names

        unsorted = [self.get_service(name) for name in service_names]
        services = [s for s in self.services if s in unsorted]

        if include_deps:
            services = reduce(self._inject_deps, services, [])

        uniques = []
        [uniques.append(s) for s in services if s not in uniques]

        return uniques

    def get_services_without_duplicate(self, service_names=None, include_deps=False):
        services = self.get_services(service_names, include_deps)
        for service in services:
            service.remove_duplicate_containers()
        return services

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

    def get_net(self, service_dict):
        net = service_dict.pop('net', None)
        if not net:
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
        containers = []
        for service in self.get_services(service_names):
            service_containers = service.start(**options)
            containers.extend(service_containers)
        return containers

    def stop(self, service_names=None, **options):
        parallel.parallel_stop(self.containers(service_names), options)

    def pause(self, service_names=None, **options):
        containers = self.containers(service_names)
        parallel.parallel_pause(reversed(containers), options)
        return containers

    def unpause(self, service_names=None, **options):
        containers = self.containers(service_names)
        parallel.parallel_unpause(containers, options)
        return containers

    def kill(self, service_names=None, **options):
        parallel.parallel_kill(self.containers(service_names), options)

    def remove_stopped(self, service_names=None, **options):
        parallel.parallel_remove(self.containers(service_names, stopped=True), options)

    def initialize_volumes(self):
        try:
            for volume in self.volumes:
                if volume.external:
                    log.debug(
                        'Volume {0} declared as external. No new '
                        'volume will be created.'.format(volume.name)
                    )
                    if not volume.exists():
                        raise ConfigurationError(
                            'Volume {name} declared as external, but could'
                            ' not be found. Please create the volume manually'
                            ' using `{command}{name}` and try again.'.format(
                                name=volume.full_name,
                                command='docker volume create --name='
                            )
                        )
                    continue
                volume.create()
        except NotFound:
            raise ConfigurationError(
                'Volume %s specifies nonexistent driver %s' % (volume.name, volume.driver)
            )
        except APIError as e:
            if 'Choose a different volume name' in str(e):
                raise ConfigurationError(
                    'Configuration for volume {0} specifies driver {1}, but '
                    'a volume with the same name uses a different driver '
                    '({3}). If you wish to use the new configuration, please '
                    'remove the existing volume "{2}" first:\n'
                    '$ docker volume rm {2}'.format(
                        volume.name, volume.driver, volume.full_name,
                        volume.inspect()['Driver']
                    )
                )

    def down(self, remove_image_type, include_volumes):
        self.stop()
        self.remove_stopped(v=include_volumes)
        self.remove_networks()

        if include_volumes:
            self.remove_volumes()

        self.remove_images(remove_image_type)

    def remove_images(self, remove_image_type):
        for service in self.get_services():
            service.remove_image(remove_image_type)

    def remove_networks(self):
        if not self.use_networking:
            return
        for network in self.networks:
            network.remove()

    def remove_volumes(self):
        for volume in self.volumes:
            volume.remove()

    def initialize_networks(self):
        if not self.use_networking:
            return

        for network in self.networks:
            network.ensure()

    def uses_default_network(self):
        return any(
            self.default_network.full_name in service.networks
            for service in self.services
        )

    @property
    def default_network(self):
        return Network(client=self.client, project=self.name, name='default')

    def restart(self, service_names=None, **options):
        containers = self.containers(service_names, stopped=True)
        parallel.parallel_restart(containers, options)
        return containers

    def build(self, service_names=None, no_cache=False, pull=False, force_rm=False):
        for service in self.get_services(service_names):
            if service.can_be_built():
                service.build(no_cache, pull, force_rm)
            else:
                log.info('%s uses an image, skipping' % service.name)

    def create(self, service_names=None, strategy=ConvergenceStrategy.changed, do_build=True):
        services = self.get_services_without_duplicate(service_names, include_deps=True)

        plans = self._get_convergence_plans(services, strategy)

        for service in services:
            service.execute_convergence_plan(
                plans[service.name],
                do_build,
                detached=True,
                start=False)

    def events(self):
        def build_container_event(event, container):
            time = datetime.datetime.fromtimestamp(event['time'])
            time = time.replace(
                microsecond=microseconds_from_time_nano(event['timeNano']))
            return {
                'time': time,
                'type': 'container',
                'action': event['status'],
                'id': container.id,
                'service': container.service,
                'attributes': {
                    'name': container.name,
                    'image': event['from'],
                }
            }

        service_names = set(self.service_names)
        for event in self.client.events(
            filters={'label': self.labels()},
            decode=True
        ):
            if event['status'] in IMAGE_EVENTS:
                # We don't receive any image events because labels aren't applied
                # to images
                continue

            # TODO: get labels from the API v1.22 , see github issue 2618
            container = Container.from_id(self.client, event['id'])
            if container.service not in service_names:
                continue
            yield build_container_event(event, container)

    def up(self,
           service_names=None,
           start_deps=True,
           strategy=ConvergenceStrategy.changed,
           do_build=True,
           timeout=DEFAULT_TIMEOUT,
           detached=False):

        services = self.get_services_without_duplicate(service_names, include_deps=start_deps)

        plans = self._get_convergence_plans(services, strategy)

        self.initialize_networks()
        self.initialize_volumes()

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
                if name in plans and
                plans[name].action in ('recreate', 'create')
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


def get_networks(service_dict, network_definitions):
    networks = []
    for name in service_dict.pop('networks', ['default']):
        if name in ['bridge', 'host']:
            networks.append(name)
        else:
            matches = [n for n in network_definitions if n.name == name]
            if matches:
                networks.append(matches[0].full_name)
            else:
                raise ConfigurationError(
                    'Service "{}" uses an undefined network "{}"'
                    .format(service_dict['name'], name))
    return networks


def get_volumes_from(project, service_dict):
    volumes_from = service_dict.pop('volumes_from', None)
    if not volumes_from:
        return []

    def build_volume_from(spec):
        if spec.type == 'service':
            try:
                return spec._replace(source=project.get_service(spec.source))
            except NoSuchService:
                pass

        if spec.type == 'container':
            try:
                container = Container.from_id(project.client, spec.source)
                return spec._replace(source=container)
            except APIError:
                pass

        raise ConfigurationError(
            "Service \"{}\" mounts volumes from \"{}\", which is not the name "
            "of a service or container.".format(
                service_dict['name'],
                spec.source))

    return [build_volume_from(vf) for vf in volumes_from]


class NoSuchService(Exception):
    def __init__(self, name):
        self.name = name
        self.msg = "No such service: %s" % self.name

    def __str__(self):
        return self.msg
