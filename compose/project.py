import datetime
import enum
import logging
import operator
import re
from functools import reduce
from os import path

from docker.errors import APIError
from docker.errors import ImageNotFound
from docker.errors import NotFound
from docker.utils import version_lt

from . import parallel
from .cli.errors import UserError
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
from .progress_stream import read_status
from .service import BuildAction
from .service import ContainerIpcMode
from .service import ContainerNetworkMode
from .service import ContainerPidMode
from .service import ConvergenceStrategy
from .service import IpcMode
from .service import NetworkMode
from .service import NoSuchImageError
from .service import parse_repository_tag
from .service import PidMode
from .service import Service
from .service import ServiceIpcMode
from .service import ServiceNetworkMode
from .service import ServicePidMode
from .utils import filter_attached_for_up
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
            labels.append('{}={}'.format(LABEL_ONE_OFF, "True"))
        elif value == cls.exclude:
            labels.append('{}={}'.format(LABEL_ONE_OFF, "False"))
        elif value == cls.include:
            pass
        else:
            raise ValueError("Invalid value for one_off: {}".format(repr(value)))


class Project:
    """
    A collection of services.
    """
    def __init__(self, name, services, client, networks=None, volumes=None, config_version=None,
                 enabled_profiles=None):
        self.name = name
        self.services = services
        self.client = client
        self.volumes = volumes or ProjectVolumes({})
        self.networks = networks or ProjectNetworks({}, False)
        self.config_version = config_version
        self.enabled_profiles = enabled_profiles or []

    def labels(self, one_off=OneOffFilter.exclude, legacy=False):
        name = self.name
        if legacy:
            name = re.sub(r'[_-]', '', name)
        labels = ['{}={}'.format(LABEL_PROJECT, name)]

        OneOffFilter.update_labels(one_off, labels)
        return labels

    @classmethod
    def from_config(cls, name, config_data, client, default_platform=None, extra_labels=None,
                    enabled_profiles=None):
        """
        Construct a Project from a config.Config object.
        """
        extra_labels = extra_labels or []
        use_networking = (config_data.version and config_data.version != V1)
        networks = build_networks(name, config_data, client)
        project_networks = ProjectNetworks.from_services(
            config_data.services,
            networks,
            use_networking)
        volumes = ProjectVolumes.from_config(name, config_data, client)
        project = cls(name, [], client, project_networks, volumes, config_data.version, enabled_profiles)

        for service_dict in config_data.services:
            service_dict = dict(service_dict)
            if use_networking:
                service_networks = get_networks(service_dict, networks)
            else:
                service_networks = {}

            service_dict.pop('networks', None)
            links = project.get_links(service_dict)
            ipc_mode = project.get_ipc_mode(service_dict)
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

            service_dict['scale'] = project.get_service_scale(service_dict)
            service_dict['device_requests'] = project.get_device_requests(service_dict)
            service_dict = translate_credential_spec_to_security_opt(service_dict)
            service_dict, ignored_keys = translate_deploy_keys_to_container_config(
                service_dict
            )
            if ignored_keys:
                log.warning(
                    'The following deploy sub-keys are not supported and have'
                    ' been ignored: {}'.format(', '.join(ignored_keys))
                )

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
                    ipc_mode=ipc_mode,
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

    def get_services(self, service_names=None, include_deps=False, auto_enable_profiles=True):
        """
        Returns a list of this project's services filtered
        by the provided list of names, or all services if service_names is None
        or [].

        If include_deps is specified, returns a list including the dependencies for
        service_names, in order of dependency.

        Preserves the original order of self.services where possible,
        reordering as needed to resolve dependencies.

        Raises NoSuchService if any of the named services do not exist.

        Raises ConfigurationError if any service depended on is not enabled by active profiles
        """
        # create a copy so we can *locally* add auto-enabled profiles later
        enabled_profiles = self.enabled_profiles.copy()

        if service_names is None or len(service_names) == 0:
            auto_enable_profiles = False
            service_names = [
                service.name
                for service in self.services
                if service.enabled_for_profiles(enabled_profiles)
            ]

        unsorted = [self.get_service(name) for name in service_names]
        services = [s for s in self.services if s in unsorted]

        if auto_enable_profiles:
            # enable profiles of explicitly targeted services
            for service in services:
                for profile in service.get_profiles():
                    if profile not in enabled_profiles:
                        enabled_profiles.append(profile)

        if include_deps:
            services = reduce(
                lambda acc, s: self._inject_deps(acc, s, enabled_profiles),
                services,
                []
            )

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

    def get_ipc_mode(self, service_dict):
        ipc_mode = service_dict.pop('ipc', None)
        if not ipc_mode:
            return IpcMode(None)

        service_name = get_service_name_from_network_mode(ipc_mode)
        if service_name:
            return ServiceIpcMode(self.get_service(service_name))

        container_name = get_container_name_from_network_mode(ipc_mode)
        if container_name:
            try:
                return ContainerIpcMode(Container.from_id(self.client, container_name))
            except APIError:
                raise ConfigurationError(
                    "Service '{name}' uses the IPC namespace of container '{dep}' which "
                    "does not exist.".format(name=service_dict['name'], dep=container_name)
                )

        return IpcMode(ipc_mode)

    def get_service_scale(self, service_dict):
        # service.scale for v2 and deploy.replicas for v3
        scale = service_dict.get('scale', None)
        deploy_dict = service_dict.get('deploy', None)
        if not deploy_dict:
            return 1 if scale is None else scale

        if deploy_dict.get('mode', 'replicated') != 'replicated':
            return 1 if scale is None else scale

        replicas = deploy_dict.get('replicas', None)
        if scale is not None and replicas is not None:
            raise ConfigurationError(
                "Both service.scale and service.deploy.replicas are set."
                " Only one of them must be set."
            )
        if replicas is not None:
            scale = replicas
        if scale is None:
            return 1
        # deploy may contain placement constraints introduced in v3.8
        max_replicas = deploy_dict.get('placement', {}).get(
            'max_replicas_per_node',
            scale)

        scale = min(scale, max_replicas)
        if max_replicas < scale:
            log.warning("Scale is limited to {} ('max_replicas_per_node' field).".format(
                max_replicas))
        return scale

    def get_device_requests(self, service_dict):
        deploy_dict = service_dict.get('deploy', None)
        if not deploy_dict:
            return

        resources = deploy_dict.get('resources', None)
        if not resources or not resources.get('reservations', None):
            return
        devices = resources['reservations'].get('devices')
        if not devices:
            return

        for dev in devices:
            count = dev.get("count", -1)
            if not isinstance(count, int):
                if count != "all":
                    raise ConfigurationError(
                        'Invalid value "{}" for devices count'.format(dev["count"]),
                        '(expected integer or "all")')
                dev["count"] = -1

            if 'capabilities' in dev:
                dev['capabilities'] = [dev['capabilities']]
        return devices

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
        for service in self.services:
            service.remove_image(remove_image_type)

    def restart(self, service_names=None, **options):
        # filter service_names by enabled profiles
        service_names = [s.name for s in self.get_services(service_names)]
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
            log.info("Building with native build. Learn about native build in Compose here: "
                     "https://docs.docker.com/go/compose-native-build/")
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
                    e.decode('utf-8') if isinstance(e, bytes) else e for e in errors.values()
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
                'attributes': {
                    k: v for k, v in container_attrs.items()
                    if not k.startswith('com.docker.compose.')
                },
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
           one_off=False,
           attach_dependencies=False,
           override_options=None,
           ):

        if cli:
            log.info("Building with native build. Learn about native build in Compose here: "
                     "https://docs.docker.com/go/compose-native-build/")

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
            services,
            strategy,
            always_recreate_deps=always_recreate_deps,
            one_off=service_names if one_off else [],
        )

        services_to_attach = filter_attached_for_up(
            services,
            service_names,
            attach_dependencies,
            lambda service: service.name)

        def do(service):
            return service.execute_convergence_plan(
                plans[service.name],
                timeout=timeout,
                detached=detached or (service not in services_to_attach),
                scale_override=scale_override.get(service.name),
                rescale=rescale,
                start=start,
                reset_container_image=reset_container_image,
                renew_anonymous_volumes=renew_anonymous_volumes,
                override_options=override_options,
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

    def _get_convergence_plans(self, services, strategy, always_recreate_deps=False, one_off=None):
        plans = {}

        for service in services:
            updated_dependencies = [
                name
                for name in service.get_dependency_names()
                if name in plans and
                plans[name].action in ('recreate', 'create')
            ]
            is_one_off = one_off and service.name in one_off

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
                    plan = service.convergence_plan(ConvergenceStrategy.always, is_one_off)
                else:
                    plan = service.convergence_plan(strategy, is_one_off)
            else:
                plan = service.convergence_plan(strategy, is_one_off)

            plans[service.name] = plan

        return plans

    def pull(self, service_names=None, ignore_pull_failures=False, parallel_pull=True, silent=False,
             include_deps=False):
        services = self.get_services(service_names, include_deps)

        if parallel_pull:
            self.parallel_pull(services, silent=silent)

        else:
            must_build = []
            for service in services:
                try:
                    service.pull(ignore_pull_failures, silent=silent)
                except (ImageNotFound, NotFound):
                    if service.can_be_built():
                        must_build.append(service.name)
                    else:
                        raise

            if len(must_build):
                log.warning('Some service image(s) must be built from source by running:\n'
                            '    docker-compose build {}'
                            .format(' '.join(must_build)))

    def parallel_pull(self, services, ignore_pull_failures=False, silent=False):
        msg = 'Pulling' if not silent else None
        must_build = []

        def pull_service(service):
            strm = service.pull(ignore_pull_failures, True, stream=True)

            if strm is None:  # Attempting to pull service with no `image` key is a no-op
                return

            try:
                writer = parallel.ParallelStreamWriter.get_instance()
                if writer is None:
                    raise RuntimeError('ParallelStreamWriter has not yet been instantiated')
                for event in strm:
                    if 'status' not in event:
                        continue
                    status = read_status(event)
                    writer.write(
                        msg, service.name, truncate_string(status), lambda s: s
                    )
            except (ImageNotFound, NotFound):
                if service.can_be_built():
                    must_build.append(service.name)
                else:
                    raise

        _, errors = parallel.parallel_execute(
            services,
            pull_service,
            operator.attrgetter('name'),
            msg,
            limit=5,
        )

        if len(must_build):
            log.warning('Some service image(s) must be built from source by running:\n'
                        '    docker-compose build {}'
                        .format(' '.join(must_build)))
        if len(errors):
            combined_errors = '\n'.join([
                e.decode('utf-8') if isinstance(e, bytes) else e for e in errors.values()
            ])
            raise ProjectError(combined_errors)

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
                log.info('Removing orphan container "{}"'.format(ctnr.name))
                try:
                    ctnr.kill()
                except APIError:
                    pass
                ctnr.remove(force=True)
        else:
            log.warning(
                'Found orphan containers ({}) for this project. If '
                'you removed or renamed this service in your compose '
                'file, you can run this command with the '
                '--remove-orphans flag to clean it up.'.format(
                    ', '.join(["{}".format(ctnr.name) for ctnr in orphans])
                )
            )

    def _inject_deps(self, acc, service, enabled_profiles):
        dep_names = service.get_dependency_names()

        if len(dep_names) > 0:
            dep_services = self.get_services(
                service_names=list(set(dep_names)),
                include_deps=True,
                auto_enable_profiles=False
            )

            for dep in dep_services:
                if not dep.enabled_for_profiles(enabled_profiles):
                    raise ConfigurationError(
                        'Service "{dep_name}" was pulled in as a dependency of '
                        'service "{service_name}" but is not enabled by the '
                        'active profiles. '
                        'You may fix this by adding a common profile to '
                        '"{dep_name}" and "{service_name}".'
                        .format(dep_name=dep.name, service_name=service.name)
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


def translate_credential_spec_to_security_opt(service_dict):
    result = []

    if 'credential_spec' in service_dict:
        spec = convert_credential_spec_to_security_opt(service_dict['credential_spec'])
        result.append('credentialspec={spec}'.format(spec=spec))

    if result:
        service_dict['security_opt'] = result

    return service_dict


def translate_resource_keys_to_container_config(resources_dict, service_dict):
    if 'limits' in resources_dict:
        service_dict['mem_limit'] = resources_dict['limits'].get('memory')
        if 'cpus' in resources_dict['limits']:
            service_dict['cpus'] = float(resources_dict['limits']['cpus'])
    if 'reservations' in resources_dict:
        service_dict['mem_reservation'] = resources_dict['reservations'].get('memory')
        if 'cpus' in resources_dict['reservations']:
            return ['resources.reservations.cpus']
    return []


def convert_restart_policy(name):
    try:
        return {
            'any': 'always',
            'none': 'no',
            'on-failure': 'on-failure'
        }[name]
    except KeyError:
        raise ConfigurationError('Invalid restart policy "{}"'.format(name))


def convert_credential_spec_to_security_opt(credential_spec):
    if 'file' in credential_spec:
        return 'file://{file}'.format(file=credential_spec['file'])
    return 'registry://{registry}'.format(registry=credential_spec['registry'])


def translate_deploy_keys_to_container_config(service_dict):
    if 'credential_spec' in service_dict:
        del service_dict['credential_spec']
    if 'configs' in service_dict:
        del service_dict['configs']

    if 'deploy' not in service_dict:
        return service_dict, []

    deploy_dict = service_dict['deploy']
    ignored_keys = [
        k for k in ['endpoint_mode', 'labels', 'update_config', 'rollback_config']
        if k in deploy_dict
    ]

    if 'restart_policy' in deploy_dict:
        service_dict['restart'] = {
            'Name': convert_restart_policy(deploy_dict['restart_policy'].get('condition', 'any')),
            'MaximumRetryCount': deploy_dict['restart_policy'].get('max_attempts', 0)
        }
        for k in deploy_dict['restart_policy'].keys():
            if k != 'condition' and k != 'max_attempts':
                ignored_keys.append('restart_policy.{}'.format(k))

    ignored_keys.extend(
        translate_resource_keys_to_container_config(
            deploy_dict.get('resources', {}), service_dict
        )
    )
    del service_dict['deploy']
    return service_dict, ignored_keys


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
            log.warning('Service "{service}" uses secret "{secret}" which is external. '
                        'External secrets are not available to containers created by '
                        'docker-compose.'.format(service=service, secret=secret.source))
            continue

        if secret.uid or secret.gid or secret.mode:
            log.warning(
                'Service "{service}" uses secret "{secret}" with uid, '
                'gid, or mode. These fields are not supported by this '
                'implementation of the Compose file'.format(
                    service=service, secret=secret.source
                )
            )

        secret_file = secret_def.get('file')
        if not path.isfile(str(secret_file)):
            log.warning(
                'Service "{service}" uses an undefined secret file "{secret_file}", '
                'the following file should be created "{secret_file}"'.format(
                    service=service, secret_file=secret_file
                )
            )
        secrets.append({'secret': secret, 'file': secret_file})

    return secrets


def get_image_digests(project):
    digests = {}
    needs_push = set()
    needs_pull = set()

    for service in project.services:
        try:
            digests[service.name] = get_image_digest(service)
        except NeedsPush as e:
            needs_push.add(e.image_name)
        except NeedsPull as e:
            needs_pull.add(e.service_name)

    if needs_push or needs_pull:
        raise MissingDigests(needs_push, needs_pull)

    return digests


def get_image_digest(service):
    if 'image' not in service.options:
        raise UserError(
            "Service '{s.name}' doesn't define an image tag. An image name is "
            "required to generate a proper image digest. Specify an image repo "
            "and tag with the 'image' option.".format(s=service))

    _, _, separator = parse_repository_tag(service.options['image'])
    # Compose file already uses a digest, no lookup required
    if separator == '@':
        return service.options['image']

    digest = get_digest(service)

    if digest:
        return digest

    if 'build' not in service.options:
        raise NeedsPull(service.image_name, service.name)

    raise NeedsPush(service.image_name)


def get_digest(service):
    digest = None
    try:
        image = service.image()
        # TODO: pick a digest based on the image tag if there are multiple
        # digests
        if image['RepoDigests']:
            digest = image['RepoDigests'][0]
    except NoSuchImageError:
        try:
            # Fetch the image digest from the registry
            distribution = service.get_image_registry_data()

            if distribution['Descriptor']['digest']:
                digest = '{image_name}@{digest}'.format(
                    image_name=service.image_name,
                    digest=distribution['Descriptor']['digest']
                )
        except NoSuchImageError:
            raise UserError(
                "Digest not found for service '{service}'. "
                "Repository does not exist or may require 'docker login'"
                .format(service=service.name))
    return digest


class MissingDigests(Exception):
    def __init__(self, needs_push, needs_pull):
        self.needs_push = needs_push
        self.needs_pull = needs_pull


class NeedsPush(Exception):
    def __init__(self, image_name):
        self.image_name = image_name


class NeedsPull(Exception):
    def __init__(self, image_name, service_name):
        self.image_name = image_name
        self.service_name = service_name


class NoSuchService(Exception):
    def __init__(self, name):
        if isinstance(name, bytes):
            name = name.decode('utf-8')
        self.name = name
        self.msg = "No such service: %s" % self.name

    def __str__(self):
        return self.msg


class ProjectError(Exception):
    def __init__(self, msg):
        self.msg = msg
