from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os
import re
import sys
from collections import namedtuple
from operator import attrgetter

import enum
import six
from docker.errors import APIError
from docker.errors import ImageNotFound
from docker.errors import NotFound
from docker.types import LogConfig
from docker.utils.ports import build_port_bindings
from docker.utils.ports import split_port
from docker.utils.utils import convert_tmpfs_mounts

from . import __version__
from . import const
from . import progress_stream
from .config import DOCKER_CONFIG_KEYS
from .config import merge_environment
from .config.errors import DependencyError
from .config.types import ServicePort
from .config.types import VolumeSpec
from .const import DEFAULT_TIMEOUT
from .const import IS_WINDOWS_PLATFORM
from .const import LABEL_CONFIG_HASH
from .const import LABEL_CONTAINER_NUMBER
from .const import LABEL_ONE_OFF
from .const import LABEL_PROJECT
from .const import LABEL_SERVICE
from .const import LABEL_VERSION
from .const import NANOCPUS_SCALE
from .container import Container
from .errors import HealthCheckFailed
from .errors import NoHealthCheckConfigured
from .errors import OperationFailedError
from .parallel import parallel_execute
from .progress_stream import stream_output
from .progress_stream import StreamOutputError
from .utils import json_hash
from .utils import parse_seconds_float


log = logging.getLogger(__name__)


HOST_CONFIG_KEYS = [
    'cap_add',
    'cap_drop',
    'cgroup_parent',
    'cpu_count',
    'cpu_percent',
    'cpu_quota',
    'cpus',
    'devices',
    'dns',
    'dns_search',
    'dns_opt',
    'env_file',
    'extra_hosts',
    'group_add',
    'init',
    'ipc',
    'read_only',
    'log_driver',
    'log_opt',
    'mem_limit',
    'mem_reservation',
    'memswap_limit',
    'mem_swappiness',
    'oom_score_adj',
    'pid',
    'pids_limit',
    'privileged',
    'restart',
    'security_opt',
    'shm_size',
    'sysctls',
    'userns_mode',
    'volumes_from',
]

CONDITION_STARTED = 'service_started'
CONDITION_HEALTHY = 'service_healthy'


class BuildError(Exception):
    def __init__(self, service, reason):
        self.service = service
        self.reason = reason


class NeedsBuildError(Exception):
    def __init__(self, service):
        self.service = service


class NoSuchImageError(Exception):
    pass


ServiceName = namedtuple('ServiceName', 'project service number')


ConvergencePlan = namedtuple('ConvergencePlan', 'action containers')


@enum.unique
class ConvergenceStrategy(enum.Enum):
    """Enumeration for all possible convergence strategies. Values refer to
    when containers should be recreated.
    """
    changed = 1
    always = 2
    never = 3

    @property
    def allows_recreate(self):
        return self is not type(self).never


@enum.unique
class ImageType(enum.Enum):
    """Enumeration for the types of images known to compose."""
    none = 0
    local = 1
    all = 2


@enum.unique
class BuildAction(enum.Enum):
    """Enumeration for the possible build actions."""
    none = 0
    force = 1
    skip = 2


class Service(object):
    def __init__(
        self,
        name,
        client=None,
        project='default',
        use_networking=False,
        links=None,
        volumes_from=None,
        network_mode=None,
        networks=None,
        secrets=None,
        scale=None,
        **options
    ):
        self.name = name
        self.client = client
        self.project = project
        self.use_networking = use_networking
        self.links = links or []
        self.volumes_from = volumes_from or []
        self.network_mode = network_mode or NetworkMode(None)
        self.networks = networks or {}
        self.secrets = secrets or []
        self.scale_num = scale or 1
        self.options = options

    def __repr__(self):
        return '<Service: {}>'.format(self.name)

    def containers(self, stopped=False, one_off=False, filters={}):
        filters.update({'label': self.labels(one_off=one_off)})

        return list(filter(None, [
            Container.from_ps(self.client, container)
            for container in self.client.containers(
                all=stopped,
                filters=filters)]))

    def get_container(self, number=1):
        """Return a :class:`compose.container.Container` for this service. The
        container must be active, and match `number`.
        """
        labels = self.labels() + ['{0}={1}'.format(LABEL_CONTAINER_NUMBER, number)]
        for container in self.client.containers(filters={'label': labels}):
            return Container.from_ps(self.client, container)

        raise ValueError("No container found for %s_%s" % (self.name, number))

    def start(self, **options):
        containers = self.containers(stopped=True)
        for c in containers:
            self.start_container_if_stopped(c, **options)
        return containers

    def show_scale_warnings(self, desired_num):
        if self.custom_container_name and desired_num > 1:
            log.warn('The "%s" service is using the custom container name "%s". '
                     'Docker requires each container to have a unique name. '
                     'Remove the custom name to scale the service.'
                     % (self.name, self.custom_container_name))

        if self.specifies_host_port() and desired_num > 1:
            log.warn('The "%s" service specifies a port on the host. If multiple containers '
                     'for this service are created on a single host, the port will clash.'
                     % self.name)

    def scale(self, desired_num, timeout=None):
        """
        Adjusts the number of containers to the specified number and ensures
        they are running.

        - creates containers until there are at least `desired_num`
        - stops containers until there are at most `desired_num` running
        - starts containers until there are at least `desired_num` running
        - removes all stopped containers
        """

        self.show_scale_warnings(desired_num)

        running_containers = self.containers(stopped=False)
        num_running = len(running_containers)

        if desired_num == num_running:
            # do nothing as we already have the desired number
            log.info('Desired container number already achieved')
            return

        if desired_num > num_running:
            all_containers = self.containers(stopped=True)

            if num_running != len(all_containers):
                # we have some stopped containers, check for divergences
                stopped_containers = [
                    c for c in all_containers if not c.is_running
                ]

                # Remove containers that have diverged
                divergent_containers = [
                    c for c in stopped_containers if self._containers_have_diverged([c])
                ]
                for c in divergent_containers:
                        c.remove()

                all_containers = list(set(all_containers) - set(divergent_containers))

            sorted_containers = sorted(all_containers, key=attrgetter('number'))
            self._execute_convergence_start(
                sorted_containers, desired_num, timeout, True, True
            )

        if desired_num < num_running:
            num_to_stop = num_running - desired_num

            sorted_running_containers = sorted(
                running_containers,
                key=attrgetter('number'))

            self._downscale(sorted_running_containers[-num_to_stop:], timeout)

    def create_container(self,
                         one_off=False,
                         previous_container=None,
                         number=None,
                         quiet=False,
                         **override_options):
        """
        Create a container for this service. If the image doesn't exist, attempt to pull
        it.
        """
        # This is only necessary for `scale` and `volumes_from`
        # auto-creating containers to satisfy the dependency.
        self.ensure_image_exists()

        container_options = self._get_container_create_options(
            override_options,
            number or self._next_container_number(one_off=one_off),
            one_off=one_off,
            previous_container=previous_container,
        )

        if 'name' in container_options and not quiet:
            log.info("Creating %s" % container_options['name'])

        try:
            return Container.create(self.client, **container_options)
        except APIError as ex:
            raise OperationFailedError("Cannot create container for service %s: %s" %
                                       (self.name, ex.explanation))

    def ensure_image_exists(self, do_build=BuildAction.none):
        if self.can_be_built() and do_build == BuildAction.force:
            self.build()
            return

        try:
            self.image()
            return
        except NoSuchImageError:
            pass

        if not self.can_be_built():
            self.pull()
            return

        if do_build == BuildAction.skip:
            raise NeedsBuildError(self)

        self.build()
        log.warn(
            "Image for service {} was built because it did not already exist. To "
            "rebuild this image you must use `docker-compose build` or "
            "`docker-compose up --build`.".format(self.name))

    def image(self):
        try:
            return self.client.inspect_image(self.image_name)
        except ImageNotFound:
            raise NoSuchImageError("Image '{}' not found".format(self.image_name))

    @property
    def image_name(self):
        return self.options.get('image', '{s.project}_{s.name}'.format(s=self))

    def convergence_plan(self, strategy=ConvergenceStrategy.changed):
        containers = self.containers(stopped=True)

        if not containers:
            return ConvergencePlan('create', [])

        if strategy is ConvergenceStrategy.never:
            return ConvergencePlan('start', containers)

        if (
            strategy is ConvergenceStrategy.always or
            self._containers_have_diverged(containers)
        ):
            return ConvergencePlan('recreate', containers)

        stopped = [c for c in containers if not c.is_running]

        if stopped:
            return ConvergencePlan('start', stopped)

        return ConvergencePlan('noop', containers)

    def _containers_have_diverged(self, containers):
        config_hash = None

        try:
            config_hash = self.config_hash
        except NoSuchImageError as e:
            log.debug(
                'Service %s has diverged: %s',
                self.name, six.text_type(e),
            )
            return True

        has_diverged = False

        for c in containers:
            container_config_hash = c.labels.get(LABEL_CONFIG_HASH, None)
            if container_config_hash != config_hash:
                log.debug(
                    '%s has diverged: %s != %s',
                    c.name, container_config_hash, config_hash,
                )
                has_diverged = True

        return has_diverged

    def _execute_convergence_create(self, scale, detached, start):
            i = self._next_container_number()

            def create_and_start(service, n):
                container = service.create_container(number=n)
                if not detached:
                    container.attach_log_stream()
                if start:
                    self.start_container(container)
                return container

            containers, errors = parallel_execute(
                range(i, i + scale),
                lambda n: create_and_start(self, n),
                lambda n: self.get_container_name(n),
                "Creating"
            )
            for error in errors.values():
                raise OperationFailedError(error)

            return containers

    def _execute_convergence_recreate(self, containers, scale, timeout, detached, start):
            if scale is not None and len(containers) > scale:
                self._downscale(containers[scale:], timeout)
                containers = containers[:scale]

            def recreate(container):
                return self.recreate_container(
                    container, timeout=timeout, attach_logs=not detached,
                    start_new_container=start
                )
            containers, errors = parallel_execute(
                containers,
                recreate,
                lambda c: c.name,
                "Recreating"
            )
            for error in errors.values():
                raise OperationFailedError(error)

            if scale is not None and len(containers) < scale:
                containers.extend(self._execute_convergence_create(
                    scale - len(containers), detached, start
                ))
            return containers

    def _execute_convergence_start(self, containers, scale, timeout, detached, start):
            if scale is not None and len(containers) > scale:
                self._downscale(containers[scale:], timeout)
                containers = containers[:scale]
            if start:
                _, errors = parallel_execute(
                    containers,
                    lambda c: self.start_container_if_stopped(c, attach_logs=not detached),
                    lambda c: c.name,
                    "Starting"
                )

                for error in errors.values():
                    raise OperationFailedError(error)

            if scale is not None and len(containers) < scale:
                containers.extend(self._execute_convergence_create(
                    scale - len(containers), detached, start
                ))
            return containers

    def _downscale(self, containers, timeout=None):
        def stop_and_remove(container):
            container.stop(timeout=self.stop_timeout(timeout))
            container.remove()

        parallel_execute(
            containers,
            stop_and_remove,
            lambda c: c.name,
            "Stopping and removing",
        )

    def execute_convergence_plan(self, plan, timeout=None, detached=False,
                                 start=True, scale_override=None, rescale=True):
        (action, containers) = plan
        scale = scale_override if scale_override is not None else self.scale_num
        containers = sorted(containers, key=attrgetter('number'))

        self.show_scale_warnings(scale)

        if action == 'create':
            return self._execute_convergence_create(
                scale, detached, start
            )

        # The create action needs always needs an initial scale, but otherwise,
        # we set scale to none in no-rescale scenarios (`run` dependencies)
        if not rescale:
            scale = None

        if action == 'recreate':
            return self._execute_convergence_recreate(
                containers, scale, timeout, detached, start
            )

        if action == 'start':
            return self._execute_convergence_start(
                containers, scale, timeout, detached, start
            )

        if action == 'noop':
            if scale != len(containers):
                return self._execute_convergence_start(
                    containers, scale, timeout, detached, start
                )
            for c in containers:
                log.info("%s is up-to-date" % c.name)

            return containers

        raise Exception("Invalid action: {}".format(action))

    def recreate_container(
            self,
            container,
            timeout=None,
            attach_logs=False,
            start_new_container=True):
        """Recreate a container.

        The original container is renamed to a temporary name so that data
        volumes can be copied to the new container, before the original
        container is removed.
        """
        log.info("Recreating %s" % container.name)

        container.stop(timeout=self.stop_timeout(timeout))
        container.rename_to_tmp_name()
        new_container = self.create_container(
            previous_container=container,
            number=container.labels.get(LABEL_CONTAINER_NUMBER),
            quiet=True,
        )
        if attach_logs:
            new_container.attach_log_stream()
        if start_new_container:
            self.start_container(new_container)
        container.remove()
        return new_container

    def stop_timeout(self, timeout):
        if timeout is not None:
            return timeout
        timeout = parse_seconds_float(self.options.get('stop_grace_period'))
        if timeout is not None:
            return timeout
        return DEFAULT_TIMEOUT

    def start_container_if_stopped(self, container, attach_logs=False, quiet=False):
        if not container.is_running:
            if not quiet:
                log.info("Starting %s" % container.name)
            if attach_logs:
                container.attach_log_stream()
            return self.start_container(container)

    def start_container(self, container):
        self.connect_container_to_networks(container)
        try:
            container.start()
        except APIError as ex:
            raise OperationFailedError("Cannot start service %s: %s" % (self.name, ex.explanation))
        return container

    def connect_container_to_networks(self, container):
        connected_networks = container.get('NetworkSettings.Networks')

        for network, netdefs in self.networks.items():
            if network in connected_networks:
                if short_id_alias_exists(container, network):
                    continue

                self.client.disconnect_container_from_network(
                    container.id,
                    network)

            self.client.connect_container_to_network(
                container.id, network,
                aliases=self._get_aliases(netdefs, container),
                ipv4_address=netdefs.get('ipv4_address', None),
                ipv6_address=netdefs.get('ipv6_address', None),
                links=self._get_links(False),
                link_local_ips=netdefs.get('link_local_ips', None),
            )

    def remove_duplicate_containers(self, timeout=None):
        for c in self.duplicate_containers():
            log.info('Removing %s' % c.name)
            c.stop(timeout=self.stop_timeout(timeout))
            c.remove()

    def duplicate_containers(self):
        containers = sorted(
            self.containers(stopped=True),
            key=lambda c: c.get('Created'),
        )

        numbers = set()

        for c in containers:
            if c.number in numbers:
                yield c
            else:
                numbers.add(c.number)

    @property
    def config_hash(self):
        return json_hash(self.config_dict())

    def config_dict(self):
        return {
            'options': self.options,
            'image_id': self.image()['Id'],
            'links': self.get_link_names(),
            'net': self.network_mode.id,
            'networks': self.networks,
            'volumes_from': [
                (v.source.name, v.mode)
                for v in self.volumes_from if isinstance(v.source, Service)
            ],
        }

    def get_dependency_names(self):
        net_name = self.network_mode.service_name
        return (
            self.get_linked_service_names() +
            self.get_volumes_from_names() +
            ([net_name] if net_name else []) +
            list(self.options.get('depends_on', {}).keys())
        )

    def get_dependency_configs(self):
        net_name = self.network_mode.service_name
        configs = dict(
            [(name, None) for name in self.get_linked_service_names()]
        )
        configs.update(dict(
            [(name, None) for name in self.get_volumes_from_names()]
        ))
        configs.update({net_name: None} if net_name else {})
        configs.update(self.options.get('depends_on', {}))
        for svc, config in self.options.get('depends_on', {}).items():
            if config['condition'] == CONDITION_STARTED:
                configs[svc] = lambda s: True
            elif config['condition'] == CONDITION_HEALTHY:
                configs[svc] = lambda s: s.is_healthy()
            else:
                # The config schema already prevents this, but it might be
                # bypassed if Compose is called programmatically.
                raise ValueError(
                    'depends_on condition "{}" is invalid.'.format(
                        config['condition']
                    )
                )

        return configs

    def get_linked_service_names(self):
        return [service.name for (service, _) in self.links]

    def get_link_names(self):
        return [(service.name, alias) for service, alias in self.links]

    def get_volumes_from_names(self):
        return [s.source.name for s in self.volumes_from if isinstance(s.source, Service)]

    # TODO: this would benefit from github.com/docker/docker/pull/14699
    # to remove the need to inspect every container
    def _next_container_number(self, one_off=False):
        containers = filter(None, [
            Container.from_ps(self.client, container)
            for container in self.client.containers(
                all=True,
                filters={'label': self.labels(one_off=one_off)})
        ])
        numbers = [c.number for c in containers]
        return 1 if not numbers else max(numbers) + 1

    def _get_aliases(self, network, container=None):
        if container and container.labels.get(LABEL_ONE_OFF) == "True":
            return []

        return list(
            {self.name} |
            ({container.short_id} if container else set()) |
            set(network.get('aliases', ()))
        )

    def build_default_networking_config(self):
        if not self.networks:
            return {}

        network = self.networks[self.network_mode.id]
        endpoint = {
            'Aliases': self._get_aliases(network),
            'IPAMConfig': {},
        }

        if network.get('ipv4_address'):
            endpoint['IPAMConfig']['IPv4Address'] = network.get('ipv4_address')
        if network.get('ipv6_address'):
            endpoint['IPAMConfig']['IPv6Address'] = network.get('ipv6_address')

        return {"EndpointsConfig": {self.network_mode.id: endpoint}}

    def _get_links(self, link_to_self):
        links = {}

        for service, link_name in self.links:
            for container in service.containers():
                links[link_name or service.name] = container.name
                links[container.name] = container.name
                links[container.name_without_project] = container.name

        if link_to_self:
            for container in self.containers():
                links[self.name] = container.name
                links[container.name] = container.name
                links[container.name_without_project] = container.name

        for external_link in self.options.get('external_links') or []:
            if ':' not in external_link:
                link_name = external_link
            else:
                external_link, link_name = external_link.split(':')
            links[link_name] = external_link

        return [
            (alias, container_name)
            for (container_name, alias) in links.items()
        ]

    def _get_volumes_from(self):
        return [build_volume_from(spec) for spec in self.volumes_from]

    def _get_container_create_options(
            self,
            override_options,
            number,
            one_off=False,
            previous_container=None):
        add_config_hash = (not one_off and not override_options)

        container_options = dict(
            (k, self.options[k])
            for k in DOCKER_CONFIG_KEYS if k in self.options)
        container_options.update(override_options)

        if not container_options.get('name'):
            container_options['name'] = self.get_container_name(number, one_off)

        container_options.setdefault('detach', True)

        # If a qualified hostname was given, split it into an
        # unqualified hostname and a domainname unless domainname
        # was also given explicitly. This matches the behavior of
        # the official Docker CLI in that scenario.
        if ('hostname' in container_options and
                'domainname' not in container_options and
                '.' in container_options['hostname']):
            parts = container_options['hostname'].partition('.')
            container_options['hostname'] = parts[0]
            container_options['domainname'] = parts[2]

        if 'ports' in container_options or 'expose' in self.options:
            container_options['ports'] = build_container_ports(
                formatted_ports(container_options.get('ports', [])),
                self.options)

        container_options['environment'] = merge_environment(
            self.options.get('environment'),
            override_options.get('environment'))

        binds, affinity = merge_volume_bindings(
            container_options.get('volumes') or [],
            self.options.get('tmpfs') or [],
            previous_container)
        override_options['binds'] = binds
        container_options['environment'].update(affinity)

        container_options['volumes'] = dict(
            (v.internal, {}) for v in container_options.get('volumes') or {})

        secret_volumes = self.get_secret_volumes()
        if secret_volumes:
            override_options['binds'].extend(v.repr() for v in secret_volumes)
            container_options['volumes'].update(
                (v.internal, {}) for v in secret_volumes)

        container_options['image'] = self.image_name

        container_options['labels'] = build_container_labels(
            container_options.get('labels', {}),
            self.labels(one_off=one_off),
            number,
            self.config_hash if add_config_hash else None)

        # Delete options which are only used in HostConfig
        for key in HOST_CONFIG_KEYS:
            container_options.pop(key, None)

        container_options['host_config'] = self._get_container_host_config(
            override_options,
            one_off=one_off)

        networking_config = self.build_default_networking_config()
        if networking_config:
            container_options['networking_config'] = networking_config

        container_options['environment'] = format_environment(
            container_options['environment'])
        return container_options

    def _get_container_host_config(self, override_options, one_off=False):
        options = dict(self.options, **override_options)

        logging_dict = options.get('logging', None)
        log_config = get_log_config(logging_dict)
        init_path = None
        if isinstance(options.get('init'), six.string_types):
            init_path = options.get('init')
            options['init'] = True

        nano_cpus = None
        if 'cpus' in options:
            nano_cpus = int(options.get('cpus') * NANOCPUS_SCALE)

        return self.client.create_host_config(
            links=self._get_links(link_to_self=one_off),
            port_bindings=build_port_bindings(
                formatted_ports(options.get('ports', []))
            ),
            binds=options.get('binds'),
            volumes_from=self._get_volumes_from(),
            privileged=options.get('privileged', False),
            network_mode=self.network_mode.mode,
            devices=options.get('devices'),
            dns=options.get('dns'),
            dns_opt=options.get('dns_opt'),
            dns_search=options.get('dns_search'),
            restart_policy=options.get('restart'),
            cap_add=options.get('cap_add'),
            cap_drop=options.get('cap_drop'),
            mem_limit=options.get('mem_limit'),
            mem_reservation=options.get('mem_reservation'),
            memswap_limit=options.get('memswap_limit'),
            ulimits=build_ulimits(options.get('ulimits')),
            log_config=log_config,
            extra_hosts=options.get('extra_hosts'),
            read_only=options.get('read_only'),
            pid_mode=options.get('pid'),
            security_opt=options.get('security_opt'),
            ipc_mode=options.get('ipc'),
            cgroup_parent=options.get('cgroup_parent'),
            cpu_quota=options.get('cpu_quota'),
            shm_size=options.get('shm_size'),
            sysctls=options.get('sysctls'),
            pids_limit=options.get('pids_limit'),
            tmpfs=options.get('tmpfs'),
            oom_score_adj=options.get('oom_score_adj'),
            mem_swappiness=options.get('mem_swappiness'),
            group_add=options.get('group_add'),
            userns_mode=options.get('userns_mode'),
            init=options.get('init', None),
            init_path=init_path,
            isolation=options.get('isolation'),
            cpu_count=options.get('cpu_count'),
            cpu_percent=options.get('cpu_percent'),
            nano_cpus=nano_cpus,
        )

    def get_secret_volumes(self):
        def build_spec(secret):
            target = '{}/{}'.format(
                const.SECRETS_PATH,
                secret['secret'].target or secret['secret'].source)
            return VolumeSpec(secret['file'], target, 'ro')

        return [build_spec(secret) for secret in self.secrets]

    def build(self, no_cache=False, pull=False, force_rm=False, build_args_override=None):
        log.info('Building %s' % self.name)

        build_opts = self.options.get('build', {})

        build_args = build_opts.get('args', {}).copy()
        if build_args_override:
            build_args.update(build_args_override)

        # python2 os.stat() doesn't support unicode on some UNIX, so we
        # encode it to a bytestring to be safe
        path = build_opts.get('context')
        if not six.PY3 and not IS_WINDOWS_PLATFORM:
            path = path.encode('utf8')

        build_output = self.client.build(
            path=path,
            tag=self.image_name,
            stream=True,
            rm=True,
            forcerm=force_rm,
            pull=pull,
            nocache=no_cache,
            dockerfile=build_opts.get('dockerfile', None),
            cache_from=build_opts.get('cache_from', None),
            labels=build_opts.get('labels', None),
            buildargs=build_args
        )

        try:
            all_events = stream_output(build_output, sys.stdout)
        except StreamOutputError as e:
            raise BuildError(self, six.text_type(e))

        # Ensure the HTTP connection is not reused for another
        # streaming command, as the Docker daemon can sometimes
        # complain about it
        self.client.close()

        image_id = None

        for event in all_events:
            if 'stream' in event:
                match = re.search(r'Successfully built ([0-9a-f]+)', event.get('stream', ''))
                if match:
                    image_id = match.group(1)

        if image_id is None:
            raise BuildError(self, event if all_events else 'Unknown')

        return image_id

    def can_be_built(self):
        return 'build' in self.options

    def labels(self, one_off=False):
        return [
            '{0}={1}'.format(LABEL_PROJECT, self.project),
            '{0}={1}'.format(LABEL_SERVICE, self.name),
            '{0}={1}'.format(LABEL_ONE_OFF, "True" if one_off else "False")
        ]

    @property
    def custom_container_name(self):
        return self.options.get('container_name')

    def get_container_name(self, number, one_off=False):
        if self.custom_container_name and not one_off:
            return self.custom_container_name

        container_name = build_container_name(
            self.project, self.name, number, one_off,
        )
        ext_links_origins = [l.split(':')[0] for l in self.options.get('external_links', [])]
        if container_name in ext_links_origins:
            raise DependencyError(
                'Service {0} has a self-referential external link: {1}'.format(
                    self.name, container_name
                )
            )
        return container_name

    def remove_image(self, image_type):
        if not image_type or image_type == ImageType.none:
            return False
        if image_type == ImageType.local and self.options.get('image'):
            return False

        log.info("Removing image %s", self.image_name)
        try:
            self.client.remove_image(self.image_name)
            return True
        except APIError as e:
            log.error("Failed to remove image for service %s: %s", self.name, e)
            return False

    def specifies_host_port(self):
        def has_host_port(binding):
            if isinstance(binding, dict):
                external_bindings = binding.get('published')
            else:
                _, external_bindings = split_port(binding)

            # there are no external bindings
            if external_bindings is None:
                return False

            # we only need to check the first binding from the range
            external_binding = external_bindings[0]

            # non-tuple binding means there is a host port specified
            if not isinstance(external_binding, tuple):
                return True

            # extract actual host port from tuple of (host_ip, host_port)
            _, host_port = external_binding
            if host_port is not None:
                return True

            return False

        return any(has_host_port(binding) for binding in self.options.get('ports', []))

    def pull(self, ignore_pull_failures=False, silent=False):
        if 'image' not in self.options:
            return

        repo, tag, separator = parse_repository_tag(self.options['image'])
        tag = tag or 'latest'
        if not silent:
            log.info('Pulling %s (%s%s%s)...' % (self.name, repo, separator, tag))
        try:
            output = self.client.pull(repo, tag=tag, stream=True)
            if silent:
                with open(os.devnull, 'w') as devnull:
                    return progress_stream.get_digest_from_pull(
                        stream_output(output, devnull))
            else:
                return progress_stream.get_digest_from_pull(
                    stream_output(output, sys.stdout))
        except (StreamOutputError, NotFound) as e:
            if not ignore_pull_failures:
                raise
            else:
                log.error(six.text_type(e))

    def push(self, ignore_push_failures=False):
        if 'image' not in self.options or 'build' not in self.options:
            return

        repo, tag, separator = parse_repository_tag(self.options['image'])
        tag = tag or 'latest'
        log.info('Pushing %s (%s%s%s)...' % (self.name, repo, separator, tag))
        output = self.client.push(repo, tag=tag, stream=True)

        try:
            return progress_stream.get_digest_from_push(
                stream_output(output, sys.stdout))
        except StreamOutputError as e:
            if not ignore_push_failures:
                raise
            else:
                log.error(six.text_type(e))

    def is_healthy(self):
        """ Check that all containers for this service report healthy.
            Returns false if at least one healthcheck is pending.
            If an unhealthy container is detected, raise a HealthCheckFailed
            exception.
        """
        result = True
        for ctnr in self.containers():
            ctnr.inspect()
            status = ctnr.get('State.Health.Status')
            if status is None:
                raise NoHealthCheckConfigured(self.name)
            elif status == 'starting':
                result = False
            elif status == 'unhealthy':
                raise HealthCheckFailed(ctnr.short_id)
        return result


def short_id_alias_exists(container, network):
    aliases = container.get(
        'NetworkSettings.Networks.{net}.Aliases'.format(net=network)) or ()
    return container.short_id in aliases


class NetworkMode(object):
    """A `standard` network mode (ex: host, bridge)"""

    service_name = None

    def __init__(self, network_mode):
        self.network_mode = network_mode

    @property
    def id(self):
        return self.network_mode

    mode = id


class ContainerNetworkMode(object):
    """A network mode that uses a container's network stack."""

    service_name = None

    def __init__(self, container):
        self.container = container

    @property
    def id(self):
        return self.container.id

    @property
    def mode(self):
        return 'container:' + self.container.id


class ServiceNetworkMode(object):
    """A network mode that uses a service's network stack."""

    def __init__(self, service):
        self.service = service

    @property
    def id(self):
        return self.service.name

    service_name = id

    @property
    def mode(self):
        containers = self.service.containers()
        if containers:
            return 'container:' + containers[0].id

        log.warn("Service %s is trying to use reuse the network stack "
                 "of another service that is not running." % (self.id))
        return None


# Names


def build_container_name(project, service, number, one_off=False):
    bits = [project, service]
    if one_off:
        bits.append('run')
    return '_'.join(bits + [str(number)])


# Images

def parse_repository_tag(repo_path):
    """Splits image identification into base image path, tag/digest
    and it's separator.

    Example:

    >>> parse_repository_tag('user/repo@sha256:digest')
    ('user/repo', 'sha256:digest', '@')
    >>> parse_repository_tag('user/repo:v1')
    ('user/repo', 'v1', ':')
    """
    tag_separator = ":"
    digest_separator = "@"

    if digest_separator in repo_path:
        repo, tag = repo_path.rsplit(digest_separator, 1)
        return repo, tag, digest_separator

    repo, tag = repo_path, ""
    if tag_separator in repo_path:
        repo, tag = repo_path.rsplit(tag_separator, 1)
        if "/" in tag:
            repo, tag = repo_path, ""

    return repo, tag, tag_separator


# Volumes


def merge_volume_bindings(volumes, tmpfs, previous_container):
    """Return a list of volume bindings for a container. Container data volumes
    are replaced by those from the previous container.
    """
    affinity = {}

    volume_bindings = dict(
        build_volume_binding(volume)
        for volume in volumes
        if volume.external)

    if previous_container:
        old_volumes = get_container_data_volumes(previous_container, volumes, tmpfs)
        warn_on_masked_volume(volumes, old_volumes, previous_container.service)
        volume_bindings.update(
            build_volume_binding(volume) for volume in old_volumes)

        if old_volumes:
            affinity = {'affinity:container': '=' + previous_container.id}

    return list(volume_bindings.values()), affinity


def get_container_data_volumes(container, volumes_option, tmpfs_option):
    """Find the container data volumes that are in `volumes_option`, and return
    a mapping of volume bindings for those volumes.
    """
    volumes = []
    volumes_option = volumes_option or []

    container_mounts = dict(
        (mount['Destination'], mount)
        for mount in container.get('Mounts') or {}
    )

    image_volumes = [
        VolumeSpec.parse(volume)
        for volume in
        container.image_config['ContainerConfig'].get('Volumes') or {}
    ]

    for volume in set(volumes_option + image_volumes):
        # No need to preserve host volumes
        if volume.external:
            continue

        # Attempting to rebind tmpfs volumes breaks: https://github.com/docker/compose/issues/4751
        if volume.internal in convert_tmpfs_mounts(tmpfs_option).keys():
            continue

        mount = container_mounts.get(volume.internal)

        # New volume, doesn't exist in the old container
        if not mount:
            continue

        # Volume was previously a host volume, now it's a container volume
        if not mount.get('Name'):
            continue

        # Copy existing volume from old container
        volume = volume._replace(external=mount['Name'])
        volumes.append(volume)

    return volumes


def warn_on_masked_volume(volumes_option, container_volumes, service):
    container_volumes = dict(
        (volume.internal, volume.external)
        for volume in container_volumes)

    for volume in volumes_option:
        if (
            volume.external and
            volume.internal in container_volumes and
            container_volumes.get(volume.internal) != volume.external
        ):
            log.warn((
                "Service \"{service}\" is using volume \"{volume}\" from the "
                "previous container. Host mapping \"{host_path}\" has no effect. "
                "Remove the existing containers (with `docker-compose rm {service}`) "
                "to use the host volume mapping."
            ).format(
                service=service,
                volume=volume.internal,
                host_path=volume.external))


def build_volume_binding(volume_spec):
    return volume_spec.internal, volume_spec.repr()


def build_volume_from(volume_from_spec):
    """
    volume_from can be either a service or a container. We want to return the
    container.id and format it into a string complete with the mode.
    """
    if isinstance(volume_from_spec.source, Service):
        containers = volume_from_spec.source.containers(stopped=True)
        if not containers:
            return "{}:{}".format(
                volume_from_spec.source.create_container().id,
                volume_from_spec.mode)

        container = containers[0]
        return "{}:{}".format(container.id, volume_from_spec.mode)
    elif isinstance(volume_from_spec.source, Container):
        return "{}:{}".format(volume_from_spec.source.id, volume_from_spec.mode)


# Labels


def build_container_labels(label_options, service_labels, number, config_hash):
    labels = dict(label_options or {})
    labels.update(label.split('=', 1) for label in service_labels)
    labels[LABEL_CONTAINER_NUMBER] = str(number)
    labels[LABEL_VERSION] = __version__

    if config_hash:
        log.debug("Added config hash: %s" % config_hash)
        labels[LABEL_CONFIG_HASH] = config_hash

    return labels


# Ulimits


def build_ulimits(ulimit_config):
    if not ulimit_config:
        return None
    ulimits = []
    for limit_name, soft_hard_values in six.iteritems(ulimit_config):
        if isinstance(soft_hard_values, six.integer_types):
            ulimits.append({'name': limit_name, 'soft': soft_hard_values, 'hard': soft_hard_values})
        elif isinstance(soft_hard_values, dict):
            ulimit_dict = {'name': limit_name}
            ulimit_dict.update(soft_hard_values)
            ulimits.append(ulimit_dict)

    return ulimits


def get_log_config(logging_dict):
    log_driver = logging_dict.get('driver', "") if logging_dict else ""
    log_options = logging_dict.get('options', None) if logging_dict else None
    return LogConfig(
        type=log_driver,
        config=log_options
    )


# TODO: remove once fix is available in docker-py
def format_environment(environment):
    def format_env(key, value):
        if value is None:
            return key
        if isinstance(value, six.binary_type):
            value = value.decode('utf-8')
        return '{key}={value}'.format(key=key, value=value)
    return [format_env(*item) for item in environment.items()]


# Ports
def formatted_ports(ports):
    result = []
    for port in ports:
        if isinstance(port, ServicePort):
            result.append(port.legacy_repr())
        else:
            result.append(port)
    return result


def build_container_ports(container_ports, options):
    ports = []
    all_ports = container_ports + options.get('expose', [])
    for port_range in all_ports:
        internal_range, _ = split_port(port_range)
        for port in internal_range:
            port = str(port)
            if '/' in port:
                port = tuple(port.split('/'))
            ports.append(port)
    return ports
