from __future__ import absolute_import
from __future__ import unicode_literals

import copy
import json
import os
import random
import shutil
import tempfile

import py
import pytest
from docker.errors import APIError
from docker.errors import NotFound

from .. import mock
from ..helpers import build_config as load_config
from ..helpers import BUSYBOX_IMAGE_WITH_TAG
from ..helpers import create_host_file
from .testcases import DockerClientTestCase
from .testcases import SWARM_SKIP_CONTAINERS_ALL
from compose.config import config
from compose.config import ConfigurationError
from compose.config import types
from compose.config.types import VolumeFromSpec
from compose.config.types import VolumeSpec
from compose.const import COMPOSEFILE_V2_0 as V2_0
from compose.const import COMPOSEFILE_V2_1 as V2_1
from compose.const import COMPOSEFILE_V2_2 as V2_2
from compose.const import COMPOSEFILE_V2_3 as V2_3
from compose.const import COMPOSEFILE_V3_1 as V3_1
from compose.const import LABEL_PROJECT
from compose.const import LABEL_SERVICE
from compose.container import Container
from compose.errors import HealthCheckFailed
from compose.errors import NoHealthCheckConfigured
from compose.project import Project
from compose.project import ProjectError
from compose.service import ConvergenceStrategy
from tests.integration.testcases import if_runtime_available
from tests.integration.testcases import is_cluster
from tests.integration.testcases import no_cluster
from tests.integration.testcases import v2_1_only
from tests.integration.testcases import v2_2_only
from tests.integration.testcases import v2_3_only
from tests.integration.testcases import v2_only
from tests.integration.testcases import v3_only


def build_config(**kwargs):
    return config.Config(
        version=kwargs.get('version'),
        services=kwargs.get('services'),
        volumes=kwargs.get('volumes'),
        networks=kwargs.get('networks'),
        secrets=kwargs.get('secrets'),
        configs=kwargs.get('configs'),
    )


class ProjectTest(DockerClientTestCase):

    def test_containers(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        project.up()

        containers = project.containers()
        assert len(containers) == 2

    @pytest.mark.skipif(SWARM_SKIP_CONTAINERS_ALL, reason='Swarm /containers/json bug')
    def test_containers_stopped(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        project.up()
        assert len(project.containers()) == 2
        assert len(project.containers(stopped=True)) == 2

        project.stop()
        assert len(project.containers()) == 0
        assert len(project.containers(stopped=True)) == 2

    def test_containers_with_service_names(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        project.up()

        containers = project.containers(['web'])
        assert len(containers) == 1
        assert containers[0].name.startswith('composetest_web_')

    def test_containers_with_extra_service(self):
        web = self.create_service('web')
        web_1 = web.create_container()

        db = self.create_service('db')
        db_1 = db.create_container()

        self.create_service('extra').create_container()

        project = Project('composetest', [web, db], self.client)
        assert set(project.containers(stopped=True)) == {web_1, db_1}

    def test_parallel_pull_with_no_image(self):
        config_data = build_config(
            version=V2_3,
            services=[{
                'name': 'web',
                'build': {'context': '.'},
            }],
        )

        project = Project.from_config(
            name='composetest',
            config_data=config_data,
            client=self.client
        )

        project.pull(parallel_pull=True)

    def test_volumes_from_service(self):
        project = Project.from_config(
            name='composetest',
            config_data=load_config({
                'data': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'volumes': ['/var/data'],
                },
                'db': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'volumes_from': ['data'],
                },
            }),
            client=self.client,
        )
        db = project.get_service('db')
        data = project.get_service('data')
        assert db.volumes_from == [VolumeFromSpec(data, 'rw', 'service')]

    def test_volumes_from_container(self):
        data_container = Container.create(
            self.client,
            image=BUSYBOX_IMAGE_WITH_TAG,
            volumes=['/var/data'],
            name='composetest_data_container',
            labels={LABEL_PROJECT: 'composetest'},
            host_config={},
        )
        project = Project.from_config(
            name='composetest',
            config_data=load_config({
                'db': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'volumes_from': ['composetest_data_container'],
                },
            }),
            client=self.client,
        )
        db = project.get_service('db')
        assert db._get_volumes_from() == [data_container.id + ':rw']

    @v2_only()
    @no_cluster('container networks not supported in Swarm')
    def test_network_mode_from_service(self):
        project = Project.from_config(
            name='composetest',
            client=self.client,
            config_data=load_config({
                'version': str(V2_0),
                'services': {
                    'net': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'command': ["top"]
                    },
                    'web': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'network_mode': 'service:net',
                        'command': ["top"]
                    },
                },
            }),
        )

        project.up()

        web = project.get_service('web')
        net = project.get_service('net')
        assert web.network_mode.mode == 'container:' + net.containers()[0].id

    @v2_only()
    @no_cluster('container networks not supported in Swarm')
    def test_network_mode_from_container(self):
        def get_project():
            return Project.from_config(
                name='composetest',
                config_data=load_config({
                    'version': str(V2_0),
                    'services': {
                        'web': {
                            'image': BUSYBOX_IMAGE_WITH_TAG,
                            'network_mode': 'container:composetest_net_container'
                        },
                    },
                }),
                client=self.client,
            )

        with pytest.raises(ConfigurationError) as excinfo:
            get_project()

        assert "container 'composetest_net_container' which does not exist" in excinfo.exconly()

        net_container = Container.create(
            self.client,
            image=BUSYBOX_IMAGE_WITH_TAG,
            name='composetest_net_container',
            command='top',
            labels={LABEL_PROJECT: 'composetest'},
            host_config={},
        )
        net_container.start()

        project = get_project()
        project.up()

        web = project.get_service('web')
        assert web.network_mode.mode == 'container:' + net_container.id

    @no_cluster('container networks not supported in Swarm')
    def test_net_from_service_v1(self):
        project = Project.from_config(
            name='composetest',
            config_data=load_config({
                'net': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"]
                },
                'web': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'net': 'container:net',
                    'command': ["top"]
                },
            }),
            client=self.client,
        )

        project.up()

        web = project.get_service('web')
        net = project.get_service('net')
        assert web.network_mode.mode == 'container:' + net.containers()[0].id

    @no_cluster('container networks not supported in Swarm')
    def test_net_from_container_v1(self):
        def get_project():
            return Project.from_config(
                name='composetest',
                config_data=load_config({
                    'web': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'net': 'container:composetest_net_container'
                    },
                }),
                client=self.client,
            )

        with pytest.raises(ConfigurationError) as excinfo:
            get_project()

        assert "container 'composetest_net_container' which does not exist" in excinfo.exconly()

        net_container = Container.create(
            self.client,
            image=BUSYBOX_IMAGE_WITH_TAG,
            name='composetest_net_container',
            command='top',
            labels={LABEL_PROJECT: 'composetest'},
            host_config={},
        )
        net_container.start()

        project = get_project()
        project.up()

        web = project.get_service('web')
        assert web.network_mode.mode == 'container:' + net_container.id

    def test_start_pause_unpause_stop_kill_remove(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        project.start()

        assert len(web.containers()) == 0
        assert len(db.containers()) == 0

        web_container_1 = web.create_container()
        web_container_2 = web.create_container()
        db_container = db.create_container()

        project.start(service_names=['web'])
        assert set(c.name for c in project.containers() if c.is_running) == {
            web_container_1.name, web_container_2.name}

        project.start()
        assert set(c.name for c in project.containers() if c.is_running) == {
            web_container_1.name, web_container_2.name, db_container.name}

        project.pause(service_names=['web'])
        assert set([c.name for c in project.containers() if c.is_paused]) == {
            web_container_1.name, web_container_2.name}

        project.pause()
        assert set([c.name for c in project.containers() if c.is_paused]) == {
            web_container_1.name, web_container_2.name, db_container.name}

        project.unpause(service_names=['db'])
        assert len([c.name for c in project.containers() if c.is_paused]) == 2

        project.unpause()
        assert len([c.name for c in project.containers() if c.is_paused]) == 0

        project.stop(service_names=['web'], timeout=1)
        assert set(c.name for c in project.containers() if c.is_running) == {db_container.name}

        project.kill(service_names=['db'])
        assert len([c for c in project.containers() if c.is_running]) == 0
        assert len(project.containers(stopped=True)) == 3

        project.remove_stopped(service_names=['web'])
        assert len(project.containers(stopped=True)) == 1

        project.remove_stopped()
        assert len(project.containers(stopped=True)) == 0

    def test_create(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)

        project.create(['db'])
        containers = project.containers(stopped=True)
        assert len(containers) == 1
        assert not containers[0].is_running
        db_containers = db.containers(stopped=True)
        assert len(db_containers) == 1
        assert not db_containers[0].is_running
        assert len(web.containers(stopped=True)) == 0

    def test_create_twice(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)

        project.create(['db', 'web'])
        project.create(['db', 'web'])
        containers = project.containers(stopped=True)
        assert len(containers) == 2
        db_containers = db.containers(stopped=True)
        assert len(db_containers) == 1
        assert not db_containers[0].is_running
        web_containers = web.containers(stopped=True)
        assert len(web_containers) == 1
        assert not web_containers[0].is_running

    def test_create_with_links(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, 'db')])
        project = Project('composetest', [db, web], self.client)

        project.create(['web'])
        # self.assertEqual(len(project.containers()), 0)
        assert len(project.containers(stopped=True)) == 2
        assert not [c for c in project.containers(stopped=True) if c.is_running]
        assert len(db.containers(stopped=True)) == 1
        assert len(web.containers(stopped=True)) == 1

    def test_create_strategy_always(self):
        db = self.create_service('db')
        project = Project('composetest', [db], self.client)
        project.create(['db'])
        old_id = project.containers(stopped=True)[0].id

        project.create(['db'], strategy=ConvergenceStrategy.always)
        assert len(project.containers(stopped=True)) == 1

        db_container = project.containers(stopped=True)[0]
        assert not db_container.is_running
        assert db_container.id != old_id

    def test_create_strategy_never(self):
        db = self.create_service('db')
        project = Project('composetest', [db], self.client)
        project.create(['db'])
        old_id = project.containers(stopped=True)[0].id

        project.create(['db'], strategy=ConvergenceStrategy.never)
        assert len(project.containers(stopped=True)) == 1

        db_container = project.containers(stopped=True)[0]
        assert not db_container.is_running
        assert db_container.id == old_id

    def test_project_up(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        assert len(project.containers()) == 0

        project.up(['db'])
        assert len(project.containers()) == 1
        assert len(db.containers()) == 1
        assert len(web.containers()) == 0

    def test_project_up_starts_uncreated_services(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, 'db')])
        project = Project('composetest', [db, web], self.client)
        project.up(['db'])
        assert len(project.containers()) == 1

        project.up()
        assert len(project.containers()) == 2
        assert len(db.containers()) == 1
        assert len(web.containers()) == 1

    def test_recreate_preserves_volumes(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/etc')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        assert len(project.containers()) == 0

        project.up(['db'])
        assert len(project.containers()) == 1
        old_db_id = project.containers()[0].id
        db_volume_path = project.containers()[0].get('Volumes./etc')

        project.up(strategy=ConvergenceStrategy.always)
        assert len(project.containers()) == 2

        db_container = [c for c in project.containers() if c.service == 'db'][0]
        assert db_container.id != old_db_id
        assert db_container.get('Volumes./etc') == db_volume_path

    @v2_3_only()
    def test_recreate_preserves_mounts(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[types.MountSpec(type='volume', target='/etc')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        assert len(project.containers()) == 0

        project.up(['db'])
        assert len(project.containers()) == 1
        old_db_id = project.containers()[0].id
        db_volume_path = project.containers()[0].get_mount('/etc')['Source']

        project.up(strategy=ConvergenceStrategy.always)
        assert len(project.containers()) == 2

        db_container = [c for c in project.containers() if c.service == 'db'][0]
        assert db_container.id != old_db_id
        assert db_container.get_mount('/etc')['Source'] == db_volume_path

    def test_project_up_with_no_recreate_running(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        assert len(project.containers()) == 0

        project.up(['db'])
        assert len(project.containers()) == 1
        container, = project.containers()
        old_db_id = container.id
        db_volume_path = container.get_mount('/var/db')['Source']

        project.up(strategy=ConvergenceStrategy.never)
        assert len(project.containers()) == 2

        db_container = [c for c in project.containers() if c.name == container.name][0]
        assert db_container.id == old_db_id
        assert db_container.get_mount('/var/db')['Source'] == db_volume_path

    def test_project_up_with_no_recreate_stopped(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        assert len(project.containers()) == 0

        project.up(['db'])
        project.kill()

        old_containers = project.containers(stopped=True)

        assert len(old_containers) == 1
        old_container, = old_containers
        old_db_id = old_container.id
        db_volume_path = old_container.get_mount('/var/db')['Source']

        project.up(strategy=ConvergenceStrategy.never)

        new_containers = project.containers(stopped=True)
        assert len(new_containers) == 2
        assert [c.is_running for c in new_containers] == [True, True]

        db_container = [c for c in new_containers if c.service == 'db'][0]
        assert db_container.id == old_db_id
        assert db_container.get_mount('/var/db')['Source'] == db_volume_path

    def test_project_up_without_all_services(self):
        console = self.create_service('console')
        db = self.create_service('db')
        project = Project('composetest', [console, db], self.client)
        project.start()
        assert len(project.containers()) == 0

        project.up()
        assert len(project.containers()) == 2
        assert len(db.containers()) == 1
        assert len(console.containers()) == 1

    def test_project_up_starts_links(self):
        console = self.create_service('console')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        web = self.create_service('web', links=[(db, 'db')])

        project = Project('composetest', [web, db, console], self.client)
        project.start()
        assert len(project.containers()) == 0

        project.up(['web'])
        assert len(project.containers()) == 2
        assert len(web.containers()) == 1
        assert len(db.containers()) == 1
        assert len(console.containers()) == 0

    def test_project_up_starts_depends(self):
        project = Project.from_config(
            name='composetest',
            config_data=load_config({
                'console': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"],
                },
                'data': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"]
                },
                'db': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"],
                    'volumes_from': ['data'],
                },
                'web': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"],
                    'links': ['db'],
                },
            }),
            client=self.client,
        )
        project.start()
        assert len(project.containers()) == 0

        project.up(['web'])
        assert len(project.containers()) == 3
        assert len(project.get_service('web').containers()) == 1
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('data').containers()) == 1
        assert len(project.get_service('console').containers()) == 0

    def test_project_up_with_no_deps(self):
        project = Project.from_config(
            name='composetest',
            config_data=load_config({
                'console': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"],
                },
                'data': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"]
                },
                'db': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"],
                    'volumes_from': ['data'],
                },
                'web': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': ["top"],
                    'links': ['db'],
                },
            }),
            client=self.client,
        )
        project.start()
        assert len(project.containers()) == 0

        project.up(['db'], start_deps=False)
        assert len(project.containers(stopped=True)) == 2
        assert len(project.get_service('web').containers()) == 0
        assert len(project.get_service('db').containers()) == 1
        assert len(project.get_service('data').containers(stopped=True)) == 1
        assert not project.get_service('data').containers(stopped=True)[0].is_running
        assert len(project.get_service('console').containers()) == 0

    def test_project_up_recreate_with_tmpfs_volume(self):
        # https://github.com/docker/compose/issues/4751
        project = Project.from_config(
            name='composetest',
            config_data=load_config({
                'version': '2.1',
                'services': {
                    'foo': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'tmpfs': ['/dev/shm'],
                        'volumes': ['/dev/shm']
                    }
                }
            }), client=self.client
        )
        project.up()
        project.up(strategy=ConvergenceStrategy.always)

    def test_unscale_after_restart(self):
        web = self.create_service('web')
        project = Project('composetest', [web], self.client)

        project.start()

        service = project.get_service('web')
        service.scale(1)
        assert len(service.containers()) == 1
        service.scale(3)
        assert len(service.containers()) == 3
        project.up()
        service = project.get_service('web')
        assert len(service.containers()) == 1
        service.scale(1)
        assert len(service.containers()) == 1
        project.up(scale_override={'web': 3})
        service = project.get_service('web')
        assert len(service.containers()) == 3
        # does scale=0 ,makes any sense? after recreating at least 1 container is running
        service.scale(0)
        project.up()
        service = project.get_service('web')
        assert len(service.containers()) == 1

    @v2_only()
    def test_project_up_networks(self):
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top',
                'networks': {
                    'foo': None,
                    'bar': None,
                    'baz': {'aliases': ['extra']},
                },
            }],
            networks={
                'foo': {'driver': 'bridge'},
                'bar': {'driver': None},
                'baz': {},
            },
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )
        project.up()

        containers = project.containers()
        assert len(containers) == 1
        container, = containers

        for net_name in ['foo', 'bar', 'baz']:
            full_net_name = 'composetest_{}'.format(net_name)
            network_data = self.client.inspect_network(full_net_name)
            assert network_data['Name'] == full_net_name

        aliases_key = 'NetworkSettings.Networks.{net}.Aliases'
        assert 'web' in container.get(aliases_key.format(net='composetest_foo'))
        assert 'web' in container.get(aliases_key.format(net='composetest_baz'))
        assert 'extra' in container.get(aliases_key.format(net='composetest_baz'))

        foo_data = self.client.inspect_network('composetest_foo')
        assert foo_data['Driver'] == 'bridge'

    @v2_only()
    def test_up_with_ipam_config(self):
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'networks': {'front': None},
            }],
            networks={
                'front': {
                    'driver': 'bridge',
                    'driver_opts': {
                        "com.docker.network.bridge.enable_icc": "false",
                    },
                    'ipam': {
                        'driver': 'default',
                        'config': [{
                            "subnet": "172.28.0.0/16",
                            "ip_range": "172.28.5.0/24",
                            "gateway": "172.28.5.254",
                            "aux_addresses": {
                                "a": "172.28.1.5",
                                "b": "172.28.1.6",
                                "c": "172.28.1.7",
                            },
                        }],
                    },
                },
            },
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )
        project.up()

        network = self.client.networks(names=['composetest_front'])[0]

        assert network['Options'] == {
            "com.docker.network.bridge.enable_icc": "false"
        }

        assert network['IPAM'] == {
            'Driver': 'default',
            'Options': None,
            'Config': [{
                'Subnet': "172.28.0.0/16",
                'IPRange': "172.28.5.0/24",
                'Gateway': "172.28.5.254",
                'AuxiliaryAddresses': {
                    'a': '172.28.1.5',
                    'b': '172.28.1.6',
                    'c': '172.28.1.7',
                },
            }],
        }

    @v2_only()
    def test_up_with_ipam_options(self):
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'networks': {'front': None},
            }],
            networks={
                'front': {
                    'driver': 'bridge',
                    'ipam': {
                        'driver': 'default',
                        'options': {
                            "com.docker.compose.network.test": "9-29-045"
                        }
                    },
                },
            },
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )
        project.up()

        network = self.client.networks(names=['composetest_front'])[0]

        assert network['IPAM']['Options'] == {
            "com.docker.compose.network.test": "9-29-045"
        }

    @v2_1_only()
    def test_up_with_network_static_addresses(self):
        config_data = build_config(
            version=V2_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top',
                'networks': {
                    'static_test': {
                        'ipv4_address': '172.16.100.100',
                        'ipv6_address': 'fe80::1001:102'
                    }
                },
            }],
            networks={
                'static_test': {
                    'driver': 'bridge',
                    'driver_opts': {
                        "com.docker.network.enable_ipv6": "true",
                    },
                    'ipam': {
                        'driver': 'default',
                        'config': [
                            {"subnet": "172.16.100.0/24",
                             "gateway": "172.16.100.1"},
                            {"subnet": "fe80::/64",
                             "gateway": "fe80::1001:1"}
                        ]
                    },
                    'enable_ipv6': True,
                }
            }
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )
        project.up(detached=True)

        service_container = project.get_service('web').containers()[0]

        ipam_config = (service_container.inspect().get('NetworkSettings', {}).
                       get('Networks', {}).get('composetest_static_test', {}).
                       get('IPAMConfig', {}))
        assert ipam_config.get('IPv4Address') == '172.16.100.100'
        assert ipam_config.get('IPv6Address') == 'fe80::1001:102'

    @v2_3_only()
    def test_up_with_network_priorities(self):
        mac_address = '74:6f:75:68:6f:75'

        def get_config_data(p1, p2, p3):
            return build_config(
                version=V2_3,
                services=[{
                    'name': 'web',
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'networks': {
                        'n1': {
                            'priority': p1,
                        },
                        'n2': {
                            'priority': p2,
                        },
                        'n3': {
                            'priority': p3,
                        }
                    },
                    'command': 'top',
                    'mac_address': mac_address
                }],
                networks={
                    'n1': {},
                    'n2': {},
                    'n3': {}
                }
            )

        config1 = get_config_data(1000, 1, 1)
        config2 = get_config_data(2, 3, 1)
        config3 = get_config_data(5, 40, 100)

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config1
        )
        project.up(detached=True)
        service_container = project.get_service('web').containers()[0]
        net_config = service_container.inspect()['NetworkSettings']['Networks']['composetest_n1']
        assert net_config['MacAddress'] == mac_address

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config2
        )
        project.up(detached=True)
        service_container = project.get_service('web').containers()[0]
        net_config = service_container.inspect()['NetworkSettings']['Networks']['composetest_n2']
        assert net_config['MacAddress'] == mac_address

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config3
        )
        project.up(detached=True)
        service_container = project.get_service('web').containers()[0]
        net_config = service_container.inspect()['NetworkSettings']['Networks']['composetest_n3']
        assert net_config['MacAddress'] == mac_address

    @v2_1_only()
    def test_up_with_enable_ipv6(self):
        self.require_api_version('1.23')
        config_data = build_config(
            version=V2_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top',
                'networks': {
                    'static_test': {
                        'ipv6_address': 'fe80::1001:102'
                    }
                },
            }],
            networks={
                'static_test': {
                    'driver': 'bridge',
                    'enable_ipv6': True,
                    'ipam': {
                        'driver': 'default',
                        'config': [
                            {"subnet": "fe80::/64",
                             "gateway": "fe80::1001:1"}
                        ]
                    }
                }
            }
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )
        project.up(detached=True)
        network = [n for n in self.client.networks() if 'static_test' in n['Name']][0]
        service_container = project.get_service('web').containers()[0]

        assert network['EnableIPv6'] is True
        ipam_config = (service_container.inspect().get('NetworkSettings', {}).
                       get('Networks', {}).get('composetest_static_test', {}).
                       get('IPAMConfig', {}))
        assert ipam_config.get('IPv6Address') == 'fe80::1001:102'

    @v2_only()
    def test_up_with_network_static_addresses_missing_subnet(self):
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'networks': {
                    'static_test': {
                        'ipv4_address': '172.16.100.100',
                        'ipv6_address': 'fe80::1001:101'
                    }
                },
            }],
            networks={
                'static_test': {
                    'driver': 'bridge',
                    'driver_opts': {
                        "com.docker.network.enable_ipv6": "true",
                    },
                    'ipam': {
                        'driver': 'default',
                    },
                },
            },
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )

        with pytest.raises(ProjectError):
            project.up()

    @v2_1_only()
    def test_up_with_network_link_local_ips(self):
        config_data = build_config(
            version=V2_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'networks': {
                    'linklocaltest': {
                        'link_local_ips': ['169.254.8.8']
                    }
                }
            }],
            networks={
                'linklocaltest': {'driver': 'bridge'}
            }
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )
        project.up(detached=True)

        service_container = project.get_service('web').containers(stopped=True)[0]
        ipam_config = service_container.inspect().get(
            'NetworkSettings', {}
        ).get(
            'Networks', {}
        ).get(
            'composetest_linklocaltest', {}
        ).get('IPAMConfig', {})
        assert 'LinkLocalIPs' in ipam_config
        assert ipam_config['LinkLocalIPs'] == ['169.254.8.8']

    @v2_1_only()
    def test_up_with_custom_name_resources(self):
        config_data = build_config(
            version=V2_2,
            services=[{
                'name': 'web',
                'volumes': [VolumeSpec.parse('foo:/container-path')],
                'networks': {'foo': {}},
                'image': BUSYBOX_IMAGE_WITH_TAG
            }],
            networks={
                'foo': {
                    'name': 'zztop',
                    'labels': {'com.docker.compose.test_value': 'sharpdressedman'}
                }
            },
            volumes={
                'foo': {
                    'name': 'acdc',
                    'labels': {'com.docker.compose.test_value': 'thefuror'}
                }
            }
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )

        project.up(detached=True)
        network = [n for n in self.client.networks() if n['Name'] == 'zztop'][0]
        volume = [v for v in self.client.volumes()['Volumes'] if v['Name'] == 'acdc'][0]

        assert network['Labels']['com.docker.compose.test_value'] == 'sharpdressedman'
        assert volume['Labels']['com.docker.compose.test_value'] == 'thefuror'

    @v2_1_only()
    def test_up_with_isolation(self):
        self.require_api_version('1.24')
        config_data = build_config(
            version=V2_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'isolation': 'default'
            }],
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )
        project.up(detached=True)
        service_container = project.get_service('web').containers(stopped=True)[0]
        assert service_container.inspect()['HostConfig']['Isolation'] == 'default'

    @v2_1_only()
    def test_up_with_invalid_isolation(self):
        self.require_api_version('1.24')
        config_data = build_config(
            version=V2_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'isolation': 'foobar'
            }],
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )
        with pytest.raises(ProjectError):
            project.up()

    @v2_3_only()
    @if_runtime_available('runc')
    def test_up_with_runtime(self):
        self.require_api_version('1.30')
        config_data = build_config(
            version=V2_3,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'runtime': 'runc'
            }],
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )
        project.up(detached=True)
        service_container = project.get_service('web').containers(stopped=True)[0]
        assert service_container.inspect()['HostConfig']['Runtime'] == 'runc'

    @v2_3_only()
    def test_up_with_invalid_runtime(self):
        self.require_api_version('1.30')
        config_data = build_config(
            version=V2_3,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'runtime': 'foobar'
            }],
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )
        with pytest.raises(ProjectError):
            project.up()

    @v2_3_only()
    @if_runtime_available('nvidia')
    def test_up_with_nvidia_runtime(self):
        self.require_api_version('1.30')
        config_data = build_config(
            version=V2_3,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'runtime': 'nvidia'
            }],
        )
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )
        project.up(detached=True)
        service_container = project.get_service('web').containers(stopped=True)[0]
        assert service_container.inspect()['HostConfig']['Runtime'] == 'nvidia'

    @v2_only()
    def test_project_up_with_network_internal(self):
        self.require_api_version('1.23')
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'networks': {'internal': None},
            }],
            networks={
                'internal': {'driver': 'bridge', 'internal': True},
            },
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )
        project.up()

        network = self.client.networks(names=['composetest_internal'])[0]

        assert network['Internal'] is True

    @v2_1_only()
    def test_project_up_with_network_label(self):
        self.require_api_version('1.23')

        network_name = 'network_with_label'

        config_data = build_config(
            version=V2_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'networks': {network_name: None}
            }],
            networks={
                network_name: {'labels': {'label_key': 'label_val'}}
            }
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data
        )

        project.up()

        networks = [
            n for n in self.client.networks()
            if n['Name'].startswith('composetest_')
        ]

        assert [n['Name'] for n in networks] == ['composetest_{}'.format(network_name)]
        assert 'label_key' in networks[0]['Labels']
        assert networks[0]['Labels']['label_key'] == 'label_val'

    @v2_only()
    def test_project_up_volumes(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={vol_name: {'driver': 'local'}},
        )

        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        project.up()
        assert len(project.containers()) == 1

        volume_data = self.get_volume_data(full_vol_name)
        assert volume_data['Name'].split('/')[-1] == full_vol_name
        assert volume_data['Driver'] == 'local'

    @v2_1_only()
    def test_project_up_with_volume_labels(self):
        self.require_api_version('1.23')

        volume_name = 'volume_with_label'

        config_data = build_config(
            version=V2_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'volumes': [VolumeSpec.parse('{}:/data'.format(volume_name))]
            }],
            volumes={
                volume_name: {
                    'labels': {
                        'label_key': 'label_val'
                    }
                }
            },
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )

        project.up()

        volumes = [
            v for v in self.client.volumes().get('Volumes', [])
            if v['Name'].split('/')[-1].startswith('composetest_')
        ]

        assert set([v['Name'].split('/')[-1] for v in volumes]) == set(
            ['composetest_{}'.format(volume_name)]
        )

        assert 'label_key' in volumes[0]['Labels']
        assert volumes[0]['Labels']['label_key'] == 'label_val'

    @v2_only()
    def test_project_up_logging_with_multiple_files(self):
        base_file = config.ConfigFile(
            'base.yml',
            {
                'version': str(V2_0),
                'services': {
                    'simple': {'image': BUSYBOX_IMAGE_WITH_TAG, 'command': 'top'},
                    'another': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'command': 'top',
                        'logging': {
                            'driver': "json-file",
                            'options': {
                                'max-size': "10m"
                            }
                        }
                    }
                }

            })
        override_file = config.ConfigFile(
            'override.yml',
            {
                'version': str(V2_0),
                'services': {
                    'another': {
                        'logging': {
                            'driver': "none"
                        }
                    }
                }

            })
        details = config.ConfigDetails('.', [base_file, override_file])

        tmpdir = py.test.ensuretemp('logging_test')
        self.addCleanup(tmpdir.remove)
        with tmpdir.as_cwd():
            config_data = config.load(details)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        project.up()
        containers = project.containers()
        assert len(containers) == 2

        another = project.get_service('another').containers()[0]
        log_config = another.get('HostConfig.LogConfig')
        assert log_config
        assert log_config.get('Type') == 'none'

    @v2_only()
    def test_project_up_port_mappings_with_multiple_files(self):
        base_file = config.ConfigFile(
            'base.yml',
            {
                'version': str(V2_0),
                'services': {
                    'simple': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'command': 'top',
                        'ports': ['1234:1234']
                    },
                },

            })
        override_file = config.ConfigFile(
            'override.yml',
            {
                'version': str(V2_0),
                'services': {
                    'simple': {
                        'ports': ['1234:1234']
                    }
                }

            })
        details = config.ConfigDetails('.', [base_file, override_file])

        config_data = config.load(details)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        project.up()
        containers = project.containers()
        assert len(containers) == 1

    @v2_2_only()
    def test_project_up_config_scale(self):
        config_data = build_config(
            version=V2_2,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top',
                'scale': 3
            }]
        )

        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        project.up()
        assert len(project.containers()) == 3

        project.up(scale_override={'web': 2})
        assert len(project.containers()) == 2

        project.up(scale_override={'web': 4})
        assert len(project.containers()) == 4

        project.stop()
        project.up()
        assert len(project.containers()) == 3

    @v2_only()
    def test_initialize_volumes(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={vol_name: {}},
        )

        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        project.volumes.initialize()

        volume_data = self.get_volume_data(full_vol_name)
        assert volume_data['Name'].split('/')[-1] == full_vol_name
        assert volume_data['Driver'] == 'local'

    @v2_only()
    def test_project_up_implicit_volume_driver(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={vol_name: {}},
        )

        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        project.up()

        volume_data = self.get_volume_data(full_vol_name)
        assert volume_data['Name'].split('/')[-1] == full_vol_name
        assert volume_data['Driver'] == 'local'

    @v3_only()
    def test_project_up_with_secrets(self):
        node = create_host_file(self.client, os.path.abspath('tests/fixtures/secrets/default'))

        config_data = build_config(
            version=V3_1,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'cat /run/secrets/special',
                'secrets': [
                    types.ServiceSecret.parse({'source': 'super', 'target': 'special'}),
                ],
                'environment': ['constraint:node=={}'.format(node if node is not None else '*')]
            }],
            secrets={
                'super': {
                    'file': os.path.abspath('tests/fixtures/secrets/default'),
                },
            },
        )

        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data,
        )
        project.up()
        project.stop()

        containers = project.containers(stopped=True)
        assert len(containers) == 1
        container, = containers

        output = container.logs()
        assert output == b"This is the secret\n"

    @v3_only()
    def test_project_up_with_added_secrets(self):
        node = create_host_file(self.client, os.path.abspath('tests/fixtures/secrets/default'))

        config_input1 = {
            'version': V3_1,
            'services': [
                {
                    'name': 'web',
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'cat /run/secrets/special',
                    'environment': ['constraint:node=={}'.format(node if node is not None else '')]
                }

            ],
            'secrets': {
                'super': {
                    'file': os.path.abspath('tests/fixtures/secrets/default')
                }
            }
        }
        config_input2 = copy.deepcopy(config_input1)
        # Add the secret
        config_input2['services'][0]['secrets'] = [
            types.ServiceSecret.parse({'source': 'super', 'target': 'special'})
        ]

        config_data1 = build_config(**config_input1)
        config_data2 = build_config(**config_input2)

        # First up with non-secret
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data1,
        )
        project.up()

        # Then up with secret
        project = Project.from_config(
            client=self.client,
            name='composetest',
            config_data=config_data2,
        )
        project.up()
        project.stop()

        containers = project.containers(stopped=True)
        assert len(containers) == 1
        container, = containers

        output = container.logs()
        assert output == b"This is the secret\n"

    @v2_only()
    def test_initialize_volumes_invalid_volume_driver(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))

        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={vol_name: {'driver': 'foobar'}},
        )

        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        with pytest.raises(APIError if is_cluster(self.client) else config.ConfigurationError):
            project.volumes.initialize()

    @v2_only()
    @no_cluster('inspect volume by name defect on Swarm Classic')
    def test_initialize_volumes_updated_driver(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)

        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={vol_name: {'driver': 'local'}},
        )
        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        project.volumes.initialize()

        volume_data = self.get_volume_data(full_vol_name)
        assert volume_data['Name'].split('/')[-1] == full_vol_name
        assert volume_data['Driver'] == 'local'

        config_data = config_data._replace(
            volumes={vol_name: {'driver': 'smb'}}
        )
        project = Project.from_config(
            name='composetest',
            config_data=config_data,
            client=self.client
        )
        with pytest.raises(config.ConfigurationError) as e:
            project.volumes.initialize()
        assert 'Configuration for volume {0} specifies driver smb'.format(
            vol_name
        ) in str(e.value)

    @v2_only()
    @no_cluster('inspect volume by name defect on Swarm Classic')
    def test_initialize_volumes_updated_driver_opts(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)
        tmpdir = tempfile.mkdtemp(prefix='compose_test_')
        self.addCleanup(shutil.rmtree, tmpdir)
        driver_opts = {'o': 'bind', 'device': tmpdir, 'type': 'none'}

        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={
                vol_name: {
                    'driver': 'local',
                    'driver_opts': driver_opts
                }
            },
        )
        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        project.volumes.initialize()

        volume_data = self.get_volume_data(full_vol_name)
        assert volume_data['Name'].split('/')[-1] == full_vol_name
        assert volume_data['Driver'] == 'local'
        assert volume_data['Options'] == driver_opts

        driver_opts['device'] = '/opt/data/localdata'
        project = Project.from_config(
            name='composetest',
            config_data=config_data,
            client=self.client
        )
        with pytest.raises(config.ConfigurationError) as e:
            project.volumes.initialize()
        assert 'Configuration for volume {0} specifies "device" driver_opt {1}'.format(
            vol_name, driver_opts['device']
        ) in str(e.value)

    @v2_only()
    def test_initialize_volumes_updated_blank_driver(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)

        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={vol_name: {'driver': 'local'}},
        )
        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        project.volumes.initialize()

        volume_data = self.get_volume_data(full_vol_name)
        assert volume_data['Name'].split('/')[-1] == full_vol_name
        assert volume_data['Driver'] == 'local'

        config_data = config_data._replace(
            volumes={vol_name: {}}
        )
        project = Project.from_config(
            name='composetest',
            config_data=config_data,
            client=self.client
        )
        project.volumes.initialize()
        volume_data = self.get_volume_data(full_vol_name)
        assert volume_data['Name'].split('/')[-1] == full_vol_name
        assert volume_data['Driver'] == 'local'

    @v2_only()
    @no_cluster('inspect volume by name defect on Swarm Classic')
    def test_initialize_volumes_external_volumes(self):
        # Use composetest_ prefix so it gets garbage-collected in tearDown()
        vol_name = 'composetest_{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)
        self.client.create_volume(vol_name)
        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={
                vol_name: {'external': True, 'name': vol_name}
            },
        )
        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        project.volumes.initialize()

        with pytest.raises(NotFound):
            self.client.inspect_volume(full_vol_name)

    @v2_only()
    def test_initialize_volumes_inexistent_external_volume(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))

        config_data = build_config(
            version=V2_0,
            services=[{
                'name': 'web',
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top'
            }],
            volumes={
                vol_name: {'external': True, 'name': vol_name}
            },
        )
        project = Project.from_config(
            name='composetest',
            config_data=config_data, client=self.client
        )
        with pytest.raises(config.ConfigurationError) as e:
            project.volumes.initialize()
        assert 'Volume {0} declared as external'.format(
            vol_name
        ) in str(e.value)

    @v2_only()
    def test_project_up_named_volumes_in_binds(self):
        vol_name = '{0:x}'.format(random.getrandbits(32))
        full_vol_name = 'composetest_{0}'.format(vol_name)

        base_file = config.ConfigFile(
            'base.yml',
            {
                'version': str(V2_0),
                'services': {
                    'simple': {
                        'image': BUSYBOX_IMAGE_WITH_TAG,
                        'command': 'top',
                        'volumes': ['{0}:/data'.format(vol_name)]
                    },
                },
                'volumes': {
                    vol_name: {'driver': 'local'}
                }

            })
        config_details = config.ConfigDetails('.', [base_file])
        config_data = config.load(config_details)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        service = project.services[0]
        assert service.name == 'simple'
        volumes = service.options.get('volumes')
        assert len(volumes) == 1
        assert volumes[0].external == full_vol_name
        project.up()
        engine_volumes = self.client.volumes()['Volumes']
        container = service.get_container()
        assert [mount['Name'] for mount in container.get('Mounts')] == [full_vol_name]
        assert next((v for v in engine_volumes if v['Name'] == vol_name), None) is None

    def test_project_up_orphans(self):
        config_dict = {
            'service1': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top',
            }
        }

        config_data = load_config(config_dict)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        project.up()
        config_dict['service2'] = config_dict['service1']
        del config_dict['service1']

        config_data = load_config(config_dict)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        with mock.patch('compose.project.log') as mock_log:
            project.up()

        mock_log.warning.assert_called_once_with(mock.ANY)

        assert len([
            ctnr for ctnr in project._labeled_containers()
            if ctnr.labels.get(LABEL_SERVICE) == 'service1'
        ]) == 1

        project.up(remove_orphans=True)

        assert len([
            ctnr for ctnr in project._labeled_containers()
            if ctnr.labels.get(LABEL_SERVICE) == 'service1'
        ]) == 0

    def test_project_up_ignore_orphans(self):
        config_dict = {
            'service1': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'top',
            }
        }

        config_data = load_config(config_dict)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        project.up()
        config_dict['service2'] = config_dict['service1']
        del config_dict['service1']

        config_data = load_config(config_dict)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        with mock.patch('compose.project.log') as mock_log:
            project.up(ignore_orphans=True)

        mock_log.warning.assert_not_called()

    @v2_1_only()
    def test_project_up_healthy_dependency(self):
        config_dict = {
            'version': '2.1',
            'services': {
                'svc1': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'healthcheck': {
                        'test': 'exit 0',
                        'retries': 1,
                        'timeout': '10s',
                        'interval': '1s'
                    },
                },
                'svc2': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'depends_on': {
                        'svc1': {'condition': 'service_healthy'},
                    }
                }
            }
        }
        config_data = load_config(config_dict)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        project.up()
        containers = project.containers()
        assert len(containers) == 2

        svc1 = project.get_service('svc1')
        svc2 = project.get_service('svc2')
        assert 'svc1' in svc2.get_dependency_names()
        assert svc1.is_healthy()

    @v2_1_only()
    def test_project_up_unhealthy_dependency(self):
        config_dict = {
            'version': '2.1',
            'services': {
                'svc1': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'healthcheck': {
                        'test': 'exit 1',
                        'retries': 1,
                        'timeout': '10s',
                        'interval': '1s'
                    },
                },
                'svc2': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'depends_on': {
                        'svc1': {'condition': 'service_healthy'},
                    }
                }
            }
        }
        config_data = load_config(config_dict)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        with pytest.raises(ProjectError):
            project.up()
        containers = project.containers()
        assert len(containers) == 1

        svc1 = project.get_service('svc1')
        svc2 = project.get_service('svc2')
        assert 'svc1' in svc2.get_dependency_names()
        with pytest.raises(HealthCheckFailed):
            svc1.is_healthy()

    @v2_1_only()
    def test_project_up_no_healthcheck_dependency(self):
        config_dict = {
            'version': '2.1',
            'services': {
                'svc1': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'healthcheck': {
                        'disable': True
                    },
                },
                'svc2': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'depends_on': {
                        'svc1': {'condition': 'service_healthy'},
                    }
                }
            }
        }
        config_data = load_config(config_dict)
        project = Project.from_config(
            name='composetest', config_data=config_data, client=self.client
        )
        with pytest.raises(ProjectError):
            project.up()
        containers = project.containers()
        assert len(containers) == 1

        svc1 = project.get_service('svc1')
        svc2 = project.get_service('svc2')
        assert 'svc1' in svc2.get_dependency_names()
        with pytest.raises(NoHealthCheckConfigured):
            svc1.is_healthy()

    def test_project_up_seccomp_profile(self):
        seccomp_data = {
            'defaultAction': 'SCMP_ACT_ALLOW',
            'syscalls': []
        }
        fd, profile_path = tempfile.mkstemp('_seccomp.json')
        self.addCleanup(os.remove, profile_path)
        with os.fdopen(fd, 'w') as f:
            json.dump(seccomp_data, f)

        config_dict = {
            'version': '2.3',
            'services': {
                'svc1': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'top',
                    'security_opt': ['seccomp:"{}"'.format(profile_path)]
                }
            }
        }

        config_data = load_config(config_dict)
        project = Project.from_config(name='composetest', config_data=config_data, client=self.client)
        project.up()
        containers = project.containers()
        assert len(containers) == 1

        remote_secopts = containers[0].get('HostConfig.SecurityOpt')
        assert len(remote_secopts) == 1
        assert remote_secopts[0].startswith('seccomp=')
        assert json.loads(remote_secopts[0].lstrip('seccomp=')) == seccomp_data

    @no_cluster('inspect volume by name defect on Swarm Classic')
    def test_project_up_name_starts_with_illegal_char(self):
        config_dict = {
            'version': '2.3',
            'services': {
                'svc1': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'ls',
                    'volumes': ['foo:/foo:rw'],
                    'networks': ['bar'],
                },
            },
            'volumes': {
                'foo': {},
            },
            'networks': {
                'bar': {},
            }
        }
        config_data = load_config(config_dict)
        project = Project.from_config(
            name='_underscoretest', config_data=config_data, client=self.client
        )
        project.up()
        self.addCleanup(project.down, None, True)

        containers = project.containers(stopped=True)
        assert len(containers) == 1
        assert containers[0].name.startswith('underscoretest_svc1_')
        assert containers[0].project == '_underscoretest'

        full_vol_name = 'underscoretest_foo'
        vol_data = self.get_volume_data(full_vol_name)
        assert vol_data
        assert vol_data['Labels'][LABEL_PROJECT] == '_underscoretest'

        full_net_name = '_underscoretest_bar'
        net_data = self.client.inspect_network(full_net_name)
        assert net_data
        assert net_data['Labels'][LABEL_PROJECT] == '_underscoretest'

        project2 = Project.from_config(
            name='-dashtest', config_data=config_data, client=self.client
        )
        project2.up()
        self.addCleanup(project2.down, None, True)

        containers = project2.containers(stopped=True)
        assert len(containers) == 1
        assert containers[0].name.startswith('dashtest_svc1_')
        assert containers[0].project == '-dashtest'

        full_vol_name = 'dashtest_foo'
        vol_data = self.get_volume_data(full_vol_name)
        assert vol_data
        assert vol_data['Labels'][LABEL_PROJECT] == '-dashtest'

        full_net_name = '-dashtest_bar'
        net_data = self.client.inspect_network(full_net_name)
        assert net_data
        assert net_data['Labels'][LABEL_PROJECT] == '-dashtest'
