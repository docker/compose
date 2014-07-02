from __future__ import unicode_literals
from __future__ import absolute_import
from fig import Service
from fig.service import CannotBeScaledError
from fig.container import Container
from fig.packages.docker.errors import APIError
from .testcases import DockerClientTestCase

class ServiceTest(DockerClientTestCase):
    def test_containers(self):
        foo = self.create_service('foo')
        bar = self.create_service('bar')

        foo.start_container()

        self.assertEqual(len(foo.containers()), 1)
        self.assertEqual(foo.containers()[0].name, 'figtest_foo_1')
        self.assertEqual(len(bar.containers()), 0)

        bar.start_container()
        bar.start_container()

        self.assertEqual(len(foo.containers()), 1)
        self.assertEqual(len(bar.containers()), 2)

        names = [c.name for c in bar.containers()]
        self.assertIn('figtest_bar_1', names)
        self.assertIn('figtest_bar_2', names)

    def test_containers_one_off(self):
        db = self.create_service('db')
        container = db.create_container(one_off=True)
        self.assertEqual(db.containers(stopped=True), [])
        self.assertEqual(db.containers(one_off=True, stopped=True), [container])

    def test_project_is_added_to_container_name(self):
        service = self.create_service('web')
        service.start_container()
        self.assertEqual(service.containers()[0].name, 'figtest_web_1')

    def test_start_stop(self):
        service = self.create_service('scalingtest')
        self.assertEqual(len(service.containers(stopped=True)), 0)

        service.create_container()
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        service.start()
        self.assertEqual(len(service.containers()), 1)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        service.stop(timeout=1)
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        service.stop(timeout=1)
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

    def test_kill_remove(self):
        service = self.create_service('scalingtest')

        service.start_container()
        self.assertEqual(len(service.containers()), 1)

        service.remove_stopped()
        self.assertEqual(len(service.containers()), 1)

        service.kill()
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        service.remove_stopped()
        self.assertEqual(len(service.containers(stopped=True)), 0)

    def test_create_container_with_one_off(self):
        db = self.create_service('db')
        container = db.create_container(one_off=True)
        self.assertEqual(container.name, 'figtest_db_run_1')

    def test_create_container_with_one_off_when_existing_container_is_running(self):
        db = self.create_service('db')
        db.start()
        container = db.create_container(one_off=True)
        self.assertEqual(container.name, 'figtest_db_run_1')

    def test_create_container_with_unspecified_volume(self):
        service = self.create_service('db', volumes=['/var/db'])
        container = service.create_container()
        service.start_container(container)
        self.assertIn('/var/db', container.inspect()['Volumes'])

    def test_create_container_with_specified_volume(self):
        service = self.create_service('db', volumes=['/tmp:/host-tmp'])
        container = service.create_container()
        service.start_container(container)
        self.assertIn('/host-tmp', container.inspect()['Volumes'])

    def test_create_container_with_volumes_from(self):
        volume_service = self.create_service('data')
        volume_container_1 = volume_service.create_container()
        volume_container_2 = Container.create(self.client, image='busybox:latest', command=["/bin/sleep", "300"])
        host_service = self.create_service('host', volumes_from=[volume_service, volume_container_2])
        host_container = host_service.create_container()
        host_service.start_container(host_container)
        self.assertIn(volume_container_1.id, host_container.inspect()['HostConfig']['VolumesFrom'])
        self.assertIn(volume_container_2.id, host_container.inspect()['HostConfig']['VolumesFrom'])

    def test_recreate_containers(self):
        service = self.create_service(
            'db',
            environment={'FOO': '1'},
            volumes=['/var/db'],
            entrypoint=['ps'],
            command=['ax']
        )
        old_container = service.create_container()
        self.assertEqual(old_container.dictionary['Config']['Entrypoint'], ['ps'])
        self.assertEqual(old_container.dictionary['Config']['Cmd'], ['ax'])
        self.assertIn('FOO=1', old_container.dictionary['Config']['Env'])
        self.assertEqual(old_container.name, 'figtest_db_1')
        service.start_container(old_container)
        volume_path = old_container.inspect()['Volumes']['/var/db']

        num_containers_before = len(self.client.containers(all=True))

        service.options['environment']['FOO'] = '2'
        tuples = service.recreate_containers()
        self.assertEqual(len(tuples), 1)

        intermediate_container = tuples[0][0]
        new_container = tuples[0][1]
        self.assertEqual(intermediate_container.dictionary['Config']['Entrypoint'], ['echo'])

        self.assertEqual(new_container.dictionary['Config']['Entrypoint'], ['ps'])
        self.assertEqual(new_container.dictionary['Config']['Cmd'], ['ax'])
        self.assertIn('FOO=2', new_container.dictionary['Config']['Env'])
        self.assertEqual(new_container.name, 'figtest_db_1')
        self.assertEqual(new_container.inspect()['Volumes']['/var/db'], volume_path)
        self.assertIn(intermediate_container.id, new_container.dictionary['HostConfig']['VolumesFrom'])

        self.assertEqual(len(self.client.containers(all=True)), num_containers_before)
        self.assertNotEqual(old_container.id, new_container.id)
        self.assertRaises(APIError, lambda: self.client.inspect_container(intermediate_container.id))

    def test_start_container_passes_through_options(self):
        db = self.create_service('db')
        db.start_container(environment={'FOO': 'BAR'})
        self.assertEqual(db.containers()[0].environment['FOO'], 'BAR')

    def test_start_container_inherits_options_from_constructor(self):
        db = self.create_service('db', environment={'FOO': 'BAR'})
        db.start_container()
        self.assertEqual(db.containers()[0].environment['FOO'], 'BAR')

    def test_start_container_creates_links(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, None)])
        db.start_container()
        web.start_container()
        self.assertIn('figtest_db_1', web.containers()[0].links())
        self.assertIn('db_1', web.containers()[0].links())

    def test_start_container_creates_links_with_names(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, 'custom_link_name')])
        db.start_container()
        web.start_container()
        self.assertIn('custom_link_name', web.containers()[0].links())

    def test_start_normal_container_does_not_create_links_to_its_own_service(self):
        db = self.create_service('db')
        c1 = db.start_container()
        c2 = db.start_container()
        self.assertNotIn(c1.name, c2.links())

    def test_start_one_off_container_creates_links_to_its_own_service(self):
        db = self.create_service('db')
        c1 = db.start_container()
        c2 = db.start_container(one_off=True)
        self.assertIn(c1.name, c2.links())

    def test_start_container_builds_images(self):
        service = Service(
            name='test',
            client=self.client,
            build='tests/fixtures/simple-dockerfile',
            project='figtest',
        )
        container = service.start_container()
        container.wait()
        self.assertIn('success', container.logs())
        self.assertEqual(len(self.client.images(name='figtest_test')), 1)

    def test_start_container_uses_tagged_image_if_it_exists(self):
        self.client.build('tests/fixtures/simple-dockerfile', tag='figtest_test')
        service = Service(
            name='test',
            client=self.client,
            build='this/does/not/exist/and/will/throw/error',
            project='figtest',
        )
        container = service.start_container()
        container.wait()
        self.assertIn('success', container.logs())

    def test_start_container_creates_ports(self):
        service = self.create_service('web', ports=[8000])
        container = service.start_container().inspect()
        self.assertEqual(list(container['NetworkSettings']['Ports'].keys()), ['8000/tcp'])
        self.assertNotEqual(container['NetworkSettings']['Ports']['8000/tcp'][0]['HostPort'], '8000')

    def test_start_container_stays_unpriviliged(self):
        service = self.create_service('web')
        container = service.start_container().inspect()
        self.assertEqual(container['HostConfig']['Privileged'], False)

    def test_start_container_becomes_priviliged(self):
        service = self.create_service('web', privileged = True)
        container = service.start_container().inspect()
        self.assertEqual(container['HostConfig']['Privileged'], True)

    def test_expose_does_not_publish_ports(self):
        service = self.create_service('web', expose=[8000])
        container = service.start_container().inspect()
        self.assertEqual(container['NetworkSettings']['Ports'], {'8000/tcp': None})

    def test_start_container_creates_port_with_explicit_protocol(self):
        service = self.create_service('web', ports=['8000/udp'])
        container = service.start_container().inspect()
        self.assertEqual(list(container['NetworkSettings']['Ports'].keys()), ['8000/udp'])

    def test_start_container_creates_fixed_external_ports(self):
        service = self.create_service('web', ports=['8000:8000'])
        container = service.start_container().inspect()
        self.assertIn('8000/tcp', container['NetworkSettings']['Ports'])
        self.assertEqual(container['NetworkSettings']['Ports']['8000/tcp'][0]['HostPort'], '8000')

    def test_start_container_creates_fixed_external_ports_when_it_is_different_to_internal_port(self):
        service = self.create_service('web', ports=['8001:8000'])
        container = service.start_container().inspect()
        self.assertIn('8000/tcp', container['NetworkSettings']['Ports'])
        self.assertEqual(container['NetworkSettings']['Ports']['8000/tcp'][0]['HostPort'], '8001')

    def test_port_with_explicit_interface(self):
        service = self.create_service('web', ports=[
            '127.0.0.1:8001:8000',
            '0.0.0.0:9001:9000',
        ])
        container = service.start_container().inspect()
        self.assertEqual(container['NetworkSettings']['Ports'], {
            '8000/tcp': [
                {
                    'HostIp': '127.0.0.1',
                    'HostPort': '8001',
                },
            ],
            '9000/tcp': [
                {
                    'HostIp': '0.0.0.0',
                    'HostPort': '9001',
                },
            ],
        })

    def test_scale(self):
        service = self.create_service('web')
        service.scale(1)
        self.assertEqual(len(service.containers()), 1)
        service.scale(3)
        self.assertEqual(len(service.containers()), 3)
        service.scale(1)
        self.assertEqual(len(service.containers()), 1)
        service.scale(0)
        self.assertEqual(len(service.containers()), 0)

    def test_scale_on_service_that_cannot_be_scaled(self):
        service = self.create_service('web', ports=['8000:8000'])
        self.assertRaises(CannotBeScaledError, lambda: service.scale(1))

    def test_scale_sets_ports(self):
        service = self.create_service('web', ports=['8000'])
        service.scale(2)
        containers = service.containers()
        self.assertEqual(len(containers), 2)
        for container in containers:
            self.assertEqual(list(container.inspect()['HostConfig']['PortBindings'].keys()), ['8000/tcp'])

    def test_network_mode_none(self):
        service = self.create_service('web', net='none')
        container = service.start_container().inspect()
        self.assertEqual(container['HostConfig']['NetworkMode'], 'none')

    def test_network_mode_bridged(self):
        service = self.create_service('web', net='bridge')
        container = service.start_container().inspect()
        self.assertEqual(container['HostConfig']['NetworkMode'], 'bridge')

    def test_network_mode_host(self):
        service = self.create_service('web', net='host')
        container = service.start_container().inspect()
        self.assertEqual(container['HostConfig']['NetworkMode'], 'host')
