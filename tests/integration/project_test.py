from __future__ import unicode_literals

from .testcases import DockerClientTestCase
from compose.cli.docker_client import docker_client
from compose.config import config
from compose.config.types import VolumeFromSpec
from compose.config.types import VolumeSpec
from compose.const import LABEL_PROJECT
from compose.container import Container
from compose.project import Project
from compose.service import ConvergenceStrategy
from compose.service import Net


def build_service_dicts(service_config):
    return config.load(
        config.ConfigDetails(
            'working_dir',
            [config.ConfigFile(None, service_config)]))


class ProjectTest(DockerClientTestCase):

    def test_containers(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        project.up()

        containers = project.containers()
        self.assertEqual(len(containers), 2)

    def test_containers_with_service_names(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        project.up()

        containers = project.containers(['web'])
        self.assertEqual(
            [c.name for c in containers],
            ['composetest_web_1'])

    def test_containers_with_extra_service(self):
        web = self.create_service('web')
        web_1 = web.create_container()

        db = self.create_service('db')
        db_1 = db.create_container()

        self.create_service('extra').create_container()

        project = Project('composetest', [web, db], self.client)
        self.assertEqual(
            set(project.containers(stopped=True)),
            set([web_1, db_1]),
        )

    def test_volumes_from_service(self):
        service_dicts = build_service_dicts({
            'data': {
                'image': 'busybox:latest',
                'volumes': ['/var/data'],
            },
            'db': {
                'image': 'busybox:latest',
                'volumes_from': ['data'],
            },
        })
        project = Project.from_dicts(
            name='composetest',
            service_dicts=service_dicts,
            client=self.client,
        )
        db = project.get_service('db')
        data = project.get_service('data')
        self.assertEqual(db.volumes_from, [VolumeFromSpec(data, 'rw')])

    def test_volumes_from_container(self):
        data_container = Container.create(
            self.client,
            image='busybox:latest',
            volumes=['/var/data'],
            name='composetest_data_container',
            labels={LABEL_PROJECT: 'composetest'},
        )
        project = Project.from_dicts(
            name='composetest',
            service_dicts=build_service_dicts({
                'db': {
                    'image': 'busybox:latest',
                    'volumes_from': ['composetest_data_container'],
                },
            }),
            client=self.client,
        )
        db = project.get_service('db')
        self.assertEqual(db._get_volumes_from(), [data_container.id + ':rw'])

    def test_get_network_does_not_exist(self):
        self.require_api_version('1.21')
        client = docker_client(version='1.21')

        project = Project('composetest', [], client)
        assert project.get_network() is None

    def test_get_network(self):
        self.require_api_version('1.21')
        client = docker_client(version='1.21')

        network_name = 'network_does_exist'
        project = Project(network_name, [], client)
        client.create_network(network_name)
        self.addCleanup(client.remove_network, network_name)
        assert project.get_network()['Name'] == network_name

    def test_net_from_service(self):
        project = Project.from_dicts(
            name='composetest',
            service_dicts=build_service_dicts({
                'net': {
                    'image': 'busybox:latest',
                    'command': ["top"]
                },
                'web': {
                    'image': 'busybox:latest',
                    'net': 'container:net',
                    'command': ["top"]
                },
            }),
            client=self.client,
        )

        project.up()

        web = project.get_service('web')
        net = project.get_service('net')
        self.assertEqual(web.net.mode, 'container:' + net.containers()[0].id)

    def test_net_from_container(self):
        net_container = Container.create(
            self.client,
            image='busybox:latest',
            name='composetest_net_container',
            command='top',
            labels={LABEL_PROJECT: 'composetest'},
        )
        net_container.start()

        project = Project.from_dicts(
            name='composetest',
            service_dicts=build_service_dicts({
                'web': {
                    'image': 'busybox:latest',
                    'net': 'container:composetest_net_container'
                },
            }),
            client=self.client,
        )

        project.up()

        web = project.get_service('web')
        self.assertEqual(web.net.mode, 'container:' + net_container.id)

    def test_start_pause_unpause_stop_kill_remove(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('composetest', [web, db], self.client)

        project.start()

        self.assertEqual(len(web.containers()), 0)
        self.assertEqual(len(db.containers()), 0)

        web_container_1 = web.create_container()
        web_container_2 = web.create_container()
        db_container = db.create_container()

        project.start(service_names=['web'])
        self.assertEqual(set(c.name for c in project.containers()), set([web_container_1.name, web_container_2.name]))

        project.start()
        self.assertEqual(set(c.name for c in project.containers()),
                         set([web_container_1.name, web_container_2.name, db_container.name]))

        project.pause(service_names=['web'])
        self.assertEqual(set([c.name for c in project.containers() if c.is_paused]),
                         set([web_container_1.name, web_container_2.name]))

        project.pause()
        self.assertEqual(set([c.name for c in project.containers() if c.is_paused]),
                         set([web_container_1.name, web_container_2.name, db_container.name]))

        project.unpause(service_names=['db'])
        self.assertEqual(len([c.name for c in project.containers() if c.is_paused]), 2)

        project.unpause()
        self.assertEqual(len([c.name for c in project.containers() if c.is_paused]), 0)

        project.stop(service_names=['web'], timeout=1)
        self.assertEqual(set(c.name for c in project.containers()), set([db_container.name]))

        project.kill(service_names=['db'])
        self.assertEqual(len(project.containers()), 0)
        self.assertEqual(len(project.containers(stopped=True)), 3)

        project.remove_stopped(service_names=['web'])
        self.assertEqual(len(project.containers(stopped=True)), 1)

        project.remove_stopped()
        self.assertEqual(len(project.containers(stopped=True)), 0)

    def test_create(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)

        project.create(['db'])
        self.assertEqual(len(project.containers()), 0)
        self.assertEqual(len(project.containers(stopped=True)), 1)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(db.containers(stopped=True)), 1)
        self.assertEqual(len(web.containers(stopped=True)), 0)

    def test_create_twice(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)

        project.create(['db', 'web'])
        project.create(['db', 'web'])
        self.assertEqual(len(project.containers()), 0)
        self.assertEqual(len(project.containers(stopped=True)), 2)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(db.containers(stopped=True)), 1)
        self.assertEqual(len(web.containers()), 0)
        self.assertEqual(len(web.containers(stopped=True)), 1)

    def test_create_with_links(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, 'db')])
        project = Project('composetest', [db, web], self.client)

        project.create(['web'])
        self.assertEqual(len(project.containers()), 0)
        self.assertEqual(len(project.containers(stopped=True)), 2)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(db.containers(stopped=True)), 1)
        self.assertEqual(len(web.containers()), 0)
        self.assertEqual(len(web.containers(stopped=True)), 1)

    def test_create_strategy_always(self):
        db = self.create_service('db')
        project = Project('composetest', [db], self.client)
        project.create(['db'])
        old_id = project.containers(stopped=True)[0].id

        project.create(['db'], strategy=ConvergenceStrategy.always)
        self.assertEqual(len(project.containers()), 0)
        self.assertEqual(len(project.containers(stopped=True)), 1)

        db_container = project.containers(stopped=True)[0]
        self.assertNotEqual(db_container.id, old_id)

    def test_create_strategy_never(self):
        db = self.create_service('db')
        project = Project('composetest', [db], self.client)
        project.create(['db'])
        old_id = project.containers(stopped=True)[0].id

        project.create(['db'], strategy=ConvergenceStrategy.never)
        self.assertEqual(len(project.containers()), 0)
        self.assertEqual(len(project.containers(stopped=True)), 1)

        db_container = project.containers(stopped=True)[0]
        self.assertEqual(db_container.id, old_id)

    def test_project_up(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        self.assertEqual(len(project.containers()), 1)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(web.containers()), 0)

    def test_project_up_starts_uncreated_services(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, 'db')])
        project = Project('composetest', [db, web], self.client)
        project.up(['db'])
        self.assertEqual(len(project.containers()), 1)

        project.up()
        self.assertEqual(len(project.containers()), 2)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(web.containers()), 1)

    def test_recreate_preserves_volumes(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/etc')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        self.assertEqual(len(project.containers()), 1)
        old_db_id = project.containers()[0].id
        db_volume_path = project.containers()[0].get('Volumes./etc')

        project.up(strategy=ConvergenceStrategy.always)
        self.assertEqual(len(project.containers()), 2)

        db_container = [c for c in project.containers() if 'db' in c.name][0]
        self.assertNotEqual(db_container.id, old_db_id)
        self.assertEqual(db_container.get('Volumes./etc'), db_volume_path)

    def test_project_up_with_no_recreate_running(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        self.assertEqual(len(project.containers()), 1)
        old_db_id = project.containers()[0].id
        db_volume_path = project.containers()[0].inspect()['Volumes']['/var/db']

        project.up(strategy=ConvergenceStrategy.never)
        self.assertEqual(len(project.containers()), 2)

        db_container = [c for c in project.containers() if 'db' in c.name][0]
        self.assertEqual(db_container.id, old_db_id)
        self.assertEqual(db_container.inspect()['Volumes']['/var/db'],
                         db_volume_path)

    def test_project_up_with_no_recreate_stopped(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        project = Project('composetest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        project.kill()

        old_containers = project.containers(stopped=True)

        self.assertEqual(len(old_containers), 1)
        old_db_id = old_containers[0].id
        db_volume_path = old_containers[0].inspect()['Volumes']['/var/db']

        project.up(strategy=ConvergenceStrategy.never)

        new_containers = project.containers(stopped=True)
        self.assertEqual(len(new_containers), 2)
        self.assertEqual([c.is_running for c in new_containers], [True, True])

        db_container = [c for c in new_containers if 'db' in c.name][0]
        self.assertEqual(db_container.id, old_db_id)
        self.assertEqual(db_container.inspect()['Volumes']['/var/db'],
                         db_volume_path)

    def test_project_up_without_all_services(self):
        console = self.create_service('console')
        db = self.create_service('db')
        project = Project('composetest', [console, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up()
        self.assertEqual(len(project.containers()), 2)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 1)

    def test_project_up_starts_links(self):
        console = self.create_service('console')
        db = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        web = self.create_service('web', links=[(db, 'db')])

        project = Project('composetest', [web, db, console], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['web'])
        self.assertEqual(len(project.containers()), 2)
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

    def test_project_up_starts_depends(self):
        project = Project.from_dicts(
            name='composetest',
            service_dicts=build_service_dicts({
                'console': {
                    'image': 'busybox:latest',
                    'command': ["top"],
                },
                'data': {
                    'image': 'busybox:latest',
                    'command': ["top"]
                },
                'db': {
                    'image': 'busybox:latest',
                    'command': ["top"],
                    'volumes_from': ['data'],
                },
                'web': {
                    'image': 'busybox:latest',
                    'command': ["top"],
                    'links': ['db'],
                },
            }),
            client=self.client,
        )
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['web'])
        self.assertEqual(len(project.containers()), 3)
        self.assertEqual(len(project.get_service('web').containers()), 1)
        self.assertEqual(len(project.get_service('db').containers()), 1)
        self.assertEqual(len(project.get_service('data').containers()), 1)
        self.assertEqual(len(project.get_service('console').containers()), 0)

    def test_project_up_with_no_deps(self):
        project = Project.from_dicts(
            name='composetest',
            service_dicts=build_service_dicts({
                'console': {
                    'image': 'busybox:latest',
                    'command': ["top"],
                },
                'data': {
                    'image': 'busybox:latest',
                    'command': ["top"]
                },
                'db': {
                    'image': 'busybox:latest',
                    'command': ["top"],
                    'volumes_from': ['data'],
                },
                'web': {
                    'image': 'busybox:latest',
                    'command': ["top"],
                    'links': ['db'],
                },
            }),
            client=self.client,
        )
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'], start_deps=False)
        self.assertEqual(len(project.containers(stopped=True)), 2)
        self.assertEqual(len(project.get_service('web').containers()), 0)
        self.assertEqual(len(project.get_service('db').containers()), 1)
        self.assertEqual(len(project.get_service('data').containers()), 0)
        self.assertEqual(len(project.get_service('data').containers(stopped=True)), 1)
        self.assertEqual(len(project.get_service('console').containers()), 0)

    def test_project_up_with_custom_network(self):
        self.require_api_version('1.21')
        client = docker_client(version='1.21')
        network_name = 'composetest-custom'

        client.create_network(network_name)
        self.addCleanup(client.remove_network, network_name)

        web = self.create_service('web', net=Net(network_name))
        project = Project('composetest', [web], client, use_networking=True)
        project.up()

        assert project.get_network() is None

    def test_unscale_after_restart(self):
        web = self.create_service('web')
        project = Project('composetest', [web], self.client)

        project.start()

        service = project.get_service('web')
        service.scale(1)
        self.assertEqual(len(service.containers()), 1)
        service.scale(3)
        self.assertEqual(len(service.containers()), 3)
        project.up()
        service = project.get_service('web')
        self.assertEqual(len(service.containers()), 3)
        service.scale(1)
        self.assertEqual(len(service.containers()), 1)
        project.up()
        service = project.get_service('web')
        self.assertEqual(len(service.containers()), 1)
        # does scale=0 ,makes any sense? after recreating at least 1 container is running
        service.scale(0)
        project.up()
        service = project.get_service('web')
        self.assertEqual(len(service.containers()), 1)
