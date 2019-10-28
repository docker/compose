from __future__ import absolute_import
from __future__ import unicode_literals

import datetime
import logging
import operator
import re
from functools import reduce
from os import path

import enum
import six
from docker.errors import APIError
from docker.utils import version_lt

from . import parallel
from .config import ConfigurationError
from .config.config import V1
from .config.sort_services import get_container_name_from_network_mode
from .config.sort_services import get_service_name_from_network_mode
from .const import LABEL_ONE_OFF
from .const import LABEL_PROJECT
from .const import LABEL_SERVICE
from .container import Container
from .network import build_networks
from .network import get_networks
from .network import ProjectNetworks
from .service import BuildAction
from .service import ContainerNetworkMode
from .service import ContainerPidMode
from .service import ConvergenceStrategy
from .service import NetworkMode
from .service import parse_repository_tag
from .service import PidMode
from .service import Service
from .service import ServiceNetworkMode
from .service import ServicePidMode
from .utils import microseconds_from_time_nano
from .utils import truncate_string
from .volume import ProjectVolumes


log = logging.getLogger(__name__)


@enum.unique
class OneOffFilter(enum.Enum):
    include = 0
    exclude = 1
    only = 2

    @classmethod
    def update_labels(cls, value, labels):
        if value == cls.only:
            labels.append('{0}={1}'.format(LABEL_ONE_OFF, "True"))
        elif value == cls.exclude:
            labels.append('{0}={1}'.format(LABEL_ONE_OFF, "False"))
        elif value == cls.include:
            pass
        else:
            raise ValueError("Invalid value for one_off: {}".format(repr(value)))


class Project(object):
    """
    A collection of services.
    """
    def __init__(self, name, services, client, networks=None, volumes=None, config_version=None):
        self.name = name
        self.services = services
        self.client = client
        self.volumes = volumes or ProjectVolumes({})
        self.networks = networks or ProjectNetworks({}, False)
        self.config_version = config_version

    def labels(self, one_off=OneOffFilter.exclude, legacy=False):
        name = self.name
        if legacy:
            name = re.sub(r'[_-]', '', name)
        labels = ['{0}={1}'.format(LABEL_PROJECT, name)]

        OneOffFilter.update_labels(one_off, labels)
        return labels

    @classmethod
    def from_config(cls, name, config_data, client, default_platform=None, extra_labels=[]):
        """
        Construct a Project from a config.Config object.
        """
        use_networking = (config_data.version and config_data.version != V1)
        networks = build_networks(name, config_data, client)
        project_networks = ProjectNetworks.from_services(
            config_data.services,
            networks,
            use_networking)
        volumes = ProjectVolumes.from_config(name, config_data, client)
        project = cls(name, [], client, project_networks, volumes, config_data.version)

        for service_dict in config_data.services:
            service_dict = dict(service_dict)
            if use_networking:
                service_networks = get_networks(service_dict, networks)
            else:
                service_networks = {}

            service_dict.pop('networks', None)
            links = project.get_links(service_dict)
            network_mode = project.get_network_mode(
                service_dict, list(service_networks.keys())
            )
            pid_mode = project.get_pid_mode(service_dict)
            volumes_from = get_volumes_from(project, service_dict)

            if config_data.version != V1:
                service_dict['volumes'] = [
                    volumes.namespace_spec(volume_spec)
                    for volume_spec in service_dict.get('volumes', [])
                ]

            secrets = get_secrets(
                service_dict['name'],
                service_dict.pop('secrets', None) or [],
                config_data.secrets)

            project.services.append(
                Service(
                    service_dict.pop('name'),
                    client=client,
                    project=name,
                    use_networking=use_networking,
                    networks=service_networks,
                    links=links,
                    network_mode=network_mode,
                    volumes_from=volumes_from,
                    secrets=secrets,
                    pid_mode=pid_mode,
                    platform=service_dict.pop('platform', None),
                    default_platform=default_platform,
                    extra_labels=extra_labels,
                    **service_dict)
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

    def get_network_mode(self, service_dict, networks):
        network_mode = service_dict.pop('network_mode', None)
        if not network_mode:
            if self.networks.use_networking:
                return NetworkMode(networks[0]) if networks else NetworkMode('none')
            return NetworkMode(None)

        service_name = get_service_name_from_network_mode(network_mode)
        if service_name:
            return ServiceNetworkMode(self.get_service(service_name))

        container_name = get_container_name_from_network_mode(network_mode)
        if container_name:
            try:
                return ContainerNetworkMode(Container.from_id(self.client, container_name))
            except APIError:
                raise ConfigurationError(
                    "Service '{name}' uses the network stack of container '{dep}' which "
                    "does not exist.".format(name=service_dict['name'], dep=container_name))

        return NetworkMode(network_mode)

    def get_pid_mode(self, service_dict):
        pid_mode = service_dict.pop('pid', None)
        if not pid_mode:
            return PidMode(None)

        service_name = get_service_name_from_network_mode(pid_mode)
        if service_name:
            return ServicePidMode(self.get_service(service_name))

        container_name = get_container_name_from_network_mode(pid_mode)
        if container_name:
            try:
                return ContainerPidMode(Container.from_id(self.client, container_name))
            except APIError:
                raise ConfigurationError(
                    "Service '{name}' uses the PID namespace of container '{dep}' which "
                    "does not exist.".format(name=service_dict['name'], dep=container_name)
                )

        return PidMode(pid_mode)

    def start(self, service_names=None, **options):
        containers = []

        def start_service(service):
            service_containers = service.start(quiet=True, **options)
            containers.extend(service_containers)

        services = self.get_services(service_names)

        def get_deps(service):
            return {
                (self.get_service(dep), config)
                for dep, config in service.get_dependency_configs().items()
            }

        parallel.parallel_execute(
            services,
            start_service,
            operator.attrgetter('name'),
            'Starting',
            get_deps,
            fail_check=lambda obj: not obj.containers(),
        )

        return containers

    def stop(self, service_names=None, one_off=OneOffFilter.exclude, **options):
        containers = self.containers(service_names, one_off=one_off)

        def get_deps(container):
            # actually returning inversed dependencies
            return {(other, None) for other in containers
                    if container.service in
                    self.get_service(other.service).get_dependency_names()}

        parallel.parallel_execute(
            containers,
            self.build_container_operation_with_timeout_func('stop', options),
            operator.attrgetter('name'),
            'Stopping',
            get_deps,
        )

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

    def remove_stopped(self, service_names=None, one_off=OneOffFilter.exclude, **options):
        parallel.parallel_remove(self.containers(
            service_names, stopped=True, one_off=one_off
        ), options)

    def down(
            self,
            remove_image_type,
            include_volumes,
            remove_orphans=False,
            timeout=None,
            ignore_orphans=False):
        self.stop(one_off=OneOffFilter.include, timeout=timeout)
        if not ignore_orphans:
            self.find_orphan_containers(remove_orphans)
        self.remove_stopped(v=include_volumes, one_off=OneOffFilter.include)

        self.networks.remove()

        if include_volumes:
            self.volumes.remove()

        self.remove_images(remove_image_type)

    def remove_images(self, remove_image_type):
        for service in self.get_services():
            service.remove_image(remove_image_type)

    def restart(self, service_names=None, **options):
        containers = self.containers(service_names, stopped=True)

        parallel.parallel_execute(
            containers,
            self.build_container_operation_with_timeout_func('restart', options),
            operator.attrgetter('name'),
            'Restarting',
        )
        return containers

    def build(self, service_names=None, no_cache=False, pull=False, force_rm=False, memory=None,
              build_args=None, gzip=False, parallel_build=False, rm=True, silent=False, cli=False,
              progress=None):

        services = []
        for service in self.get_services(service_names):
            if service.can_be_built():
                services.append(service)
            elif not silent:
                log.info('%s uses an image, skipping' % service.name)

        if cli:
            log.warning("Native build is an experimental feature and could change at any time")
            if parallel_build:
                log.warning("Flag '--parallel' is ignored when building with "
                            "COMPOSE_DOCKER_CLI_BUILD=1")
            if gzip:
                log.warning("Flag '--compress' is ignored when building with "
                            "COMPOSE_DOCKER_CLI_BUILD=1")

        def build_service(service):
            service.build(no_cache, pull, force_rm, memory, build_args, gzip, rm, silent, cli, progress)
        if parallel_build:
            _, errors = parallel.parallel_execute(
                services,
                build_service,
                operator.attrgetter('name'),
                'Building',
                limit=5,
            )
            if len(errors):
                combined_errors = '\n'.join([
                    e.decode('utf-8') if isinstance(e, six.binary_type) else e for e in errors.values()
                ])
                raise ProjectError(combined_errors)

        else:
            for service in services:
                build_service(service)

    def create(
        self,
        service_names=None,
        strategy=ConvergenceStrategy.changed,
        do_build=BuildAction.none,
    ):
        services = self.get_services_without_duplicate(service_names, include_deps=True)

        for svc in services:
            svc.ensure_image_exists(do_build=do_build)
        plans = self._get_convergence_plans(services, strategy)

        for service in services:
            service.execute_convergence_plan(
                plans[service.name],
                detached=True,
                start=False)

    def _legacy_event_processor(self, service_names):
        # Only for v1 files or when Compose is forced to use an older API version
        def build_container_event(event, container):
            time = datetime.datetime.fromtimestamp(event['time'])
            time = time.replace(
                microsecond=microseconds_from_time_nano(event['timeNano'])
            )
            return {
                'time': time,
                'type': 'container',
                'action': event['status'],
                'id': container.id,
                'service': container.service,
                'attributes': {
                    'name': container.name,
                    'image': event['from'],
                },
                'container': container,
            }

        service_names = set(service_names or self.service_names)
        for event in self.client.events(
            filters={'label': self.labels()},
            decode=True
        ):
            # This is a guard against some events broadcasted by swarm that
            # don't have a status field.
            # See https://github.com/docker/compose/issues/3316
            if 'status' not in event:
                continue

            try:
                # this can fail if the container has been removed or if the event
                # refers to an image
                container = Container.from_id(self.client, event['id'])
            except APIError:
                continue
            if container.service not in service_names:
                continue
            yield build_container_event(event, container)

    def events(self, service_names=None):
        if version_lt(self.client.api_version, '1.22'):
            # New, better event API was introduced in 1.22.
            return self._legacy_event_processor(service_names)

        def build_container_event(event):
            container_attrs = event['Actor']['Attributes']
            time = datetime.datetime.fromtimestamp(event['time'])
            time = time.replace(
                microsecond=microseconds_from_time_nano(event['timeNano'])
            )

            container = None
            try:
                container = Container.from_id(self.client, event['id'])
            except APIError:
                # Container may have been removed (e.g. if this is a destroy event)
                pass

            return {
                'time': time,
                'type': 'container',
                'action': event['status'],
                'id': event['Actor']['ID'],
                'service': container_attrs.get(LABEL_SERVICE),
                'attributes': dict([
                    (k, v) for k, v in container_attrs.items()
                    if not k.startswith('com.docker.compose.')
                ]),
                'container': container,
            }

        def yield_loop(service_names):
            for event in self.client.events(
                filters={'label': self.labels()},
                decode=True
            ):
                # TODO: support other event types
                if event.get('Type') != 'container':
                    continue

                try:
                    if event['Actor']['Attributes'][LABEL_SERVICE] not in service_names:
                        continue
                except KeyError:
                    continue
                yield build_container_event(event)

        return yield_loop(set(service_names) if service_names else self.service_names)

    def up(self,
           service_names=None,
           start_deps=True,
           strategy=ConvergenceStrategy.changed,
           do_build=BuildAction.none,
           timeout=None,
           detached=False,
           remove_orphans=False,
           ignore_orphans=False,
           scale_override=None,
           rescale=True,
           start=True,
           always_recreate_deps=False,
           reset_container_image=False,
           renew_anonymous_volumes=False,
           silent=False,
           cli=False,
           ):

        if cli:
            log.warning("Native build is an experimental feature and could change at any time")

        self.initialize()
        if not ignore_orphans:
            self.find_orphan_containers(remove_orphans)

        if scale_override is None:
            scale_override = {}

        services = self.get_services_without_duplicate(
            service_names,
            include_deps=start_deps)

        for svc in services:
            svc.ensure_image_exists(do_build=do_build, silent=silent, cli=cli)
        plans = self._get_convergence_plans(
            services, strategy, always_recreate_deps=always_recreate_deps)

        def do(service):

            return service.execute_convergence_plan(
                plans[service.name],
                timeout=timeout,
                detached=detached,
                scale_override=scale_override.get(service.name),
                rescale=rescale,
                start=start,
                reset_container_image=reset_container_image,
                renew_anonymous_volumes=renew_anonymous_volumes,
            )

        def get_deps(service):
            return {
                (self.get_service(dep), config)
                for dep, config in service.get_dependency_configs().items()
            }

        results, errors = parallel.parallel_execute(
            services,
            do,
            operator.attrgetter('name'),
            None,
            get_deps,
        )
        if errors:
            raise ProjectError(
                'Encountered errors while bringing up the project.'
            )

        return [
            container
            for svc_containers in results
            if svc_containers is not None
            for container in svc_containers
        ]

    def initialize(self):
        self.networks.initialize()
        self.volumes.initialize()

    def _get_convergence_plans(self, services, strategy, always_recreate_deps=False):
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
                containers_stopped = any(
                    service.containers(stopped=True, filters={'status': ['created', 'exited']}))
                service_has_links = any(service.get_link_names())
                container_has_links = any(c.get('HostConfig.Links') for c in service.containers())
                should_recreate_for_links = service_has_links ^ container_has_links
                if always_recreate_deps or containers_stopped or should_recreate_for_links:
                    plan = service.convergence_plan(ConvergenceStrategy.always)
                else:
                    plan = service.convergence_plan(strategy)
            else:
                plan = service.convergence_plan(strategy)

            plans[service.name] = plan

        return plans

    def pull(self, service_names=None, ignore_pull_failures=False, parallel_pull=False, silent=False,
             include_deps=False):
        services = self.get_services(service_names, include_deps)
        images_to_build = {service.image_name for service in services if service.can_be_built()}
        services_to_pull = [service for service in services if service.image_name not in images_to_build]

        msg = not silent and 'Pulling' or None

        if parallel_pull:
            def pull_service(service):
                strm = service.pull(ignore_pull_failures, True, stream=True)
                if strm is None:  # Attempting to pull service with no `image` key is a no-op
                    return

                writer = parallel.get_stream_writer()

                for event in strm:
                    if 'status' not in event:
                        continue
                    status = event['status'].lower()
                    if 'progressDetail' in event:
                        detail = event['progressDetail']
                        if 'current' in detail and 'total' in detail:
                            percentage = float(detail['current']) / float(detail['total'])
                            status = '{} ({:.1%})'.format(status, percentage)

                    writer.write(
                        msg, service.name, truncate_string(status), lambda s: s
                    )

            _, errors = parallel.parallel_execute(
                services_to_pull,
                pull_service,
                operator.attrgetter('name'),
                msg,
                limit=5,
            )
            if len(errors):
                combined_errors = '\n'.join([
                    e.decode('utf-8') if isinstance(e, six.binary_type) else e for e in errors.values()
                ])
                raise ProjectError(combined_errors)

        else:
            for service in services_to_pull:
                service.pull(ignore_pull_failures, silent=silent)

    def push(self, service_names=None, ignore_push_failures=False):
        unique_images = set()
        for service in self.get_services(service_names, include_deps=False):
            # Considering <image> and <image:latest> as the same
            repo, tag, sep = parse_repository_tag(service.image_name)
            service_image_name = sep.join((repo, tag)) if tag else sep.join((repo, 'latest'))

            if service_image_name not in unique_images:
                service.push(ignore_push_failures)
                unique_images.add(service_image_name)

    def _labeled_containers(self, stopped=False, one_off=OneOffFilter.exclude):
        ctnrs = list(filter(None, [
            Container.from_ps(self.client, container)
            for container in self.client.containers(
                all=stopped,
                filters={'label': self.labels(one_off=one_off)})])
        )
        if ctnrs:
            return ctnrs

        return list(filter(lambda c: c.has_legacy_proj_name(self.name), filter(None, [
            Container.from_ps(self.client, container)
            for container in self.client.containers(
                all=stopped,
                filters={'label': self.labels(one_off=one_off, legacy=True)})])
        ))

    def containers(self, service_names=None, stopped=False, one_off=OneOffFilter.exclude):
        if service_names:
            self.validate_service_names(service_names)
        else:
            service_names = self.service_names

        containers = self._labeled_containers(stopped, one_off)

        def matches_service_names(container):
            return container.labels.get(LABEL_SERVICE) in service_names

        return [c for c in containers if matches_service_names(c)]

    def find_orphan_containers(self, remove_orphans):
        def _find():
            containers = set(self._labeled_containers() + self._labeled_containers(stopped=True))
            for ctnr in containers:
                service_name = ctnr.labels.get(LABEL_SERVICE)
                if service_name not in self.service_names:
                    yield ctnr
        orphans = list(_find())
        if not orphans:
            return
        if remove_orphans:
            for ctnr in orphans:
                log.info('Removing orphan container "{0}"'.format(ctnr.name))
                try:
                    ctnr.kill()
                except APIError:
                    pass
                ctnr.remove(force=True)
        else:
            log.warning(
                'Found orphan containers ({0}) for this project. If '
                'you removed or renamed this service in your compose '
                'file, you can run this command with the '
                '--remove-orphans flag to clean it up.'.format(
                    ', '.join(["{}".format(ctnr.name) for ctnr in orphans])
                )
            )

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

    def build_container_operation_with_timeout_func(self, operation, options):
        def container_operation_with_timeout(container):
            _options = options.copy()
            if _options.get('timeout') is None:
                service = self.get_service(container.service)
                _options['timeout'] = service.stop_timeout(None)
            return getattr(container, operation)(**_options)
        return container_operation_with_timeout


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


def get_secrets(service, service_secrets, secret_defs):
    secrets = []

    for secret in service_secrets:
        secret_def = secret_defs.get(secret.source)
        if not secret_def:
            raise ConfigurationError(
                "Service \"{service}\" uses an undefined secret \"{secret}\" "
                .format(service=service, secret=secret.source))

        if secret_def.get('external'):
            log.warning("Service \"{service}\" uses secret \"{secret}\" which is external. "
                        "External secrets are not available to containers created by "
                        "docker-compose.".format(service=service, secret=secret.source))
            continue

        if secret.uid or secret.gid or secret.mode:
            log.warning(
                "Service \"{service}\" uses secret \"{secret}\" with uid, "
                "gid, or mode. These fields are not supported by this "
                "implementation of the Compose file".format(
                    service=service, secret=secret.source
                )
            )

        secret_file = secret_def.get('file')
        if not path.isfile(str(secret_file)):
            log.warning(
                "Service \"{service}\" uses an undefined secret file \"{secret_file}\", "
                "the following file should be created \"{secret_file}\"".format(
                    service=service, secret_file=secret_file
                )
            )
        secrets.append({'secret': secret, 'file': secret_file})

    return secrets


class NoSuchService(Exception):
    def __init__(self, name):
        if isinstance(name, six.binary_type):
            name = name.decode('utf-8')
        self.name = name
        self.msg = "No such service: %s" % self.name

    def __str__(self):
        return self.msg


class ProjectError(Exception):
    def __init__(self, msg):
        self.msg = msg
