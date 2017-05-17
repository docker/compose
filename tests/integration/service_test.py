from __future__ import absolute_import
from __future__ import unicode_literals

import os
import shutil
import tempfile
from distutils.spawn import find_executable
from os import path

import pytest
from docker.errors import APIError
from six import StringIO
from six import text_type

from .. import mock
from .testcases import DockerClientTestCase
from .testcases import get_links
from .testcases import pull_busybox
from compose import __version__
from compose.config.types import VolumeFromSpec
from compose.config.types import VolumeSpec
from compose.const import IS_WINDOWS_PLATFORM
from compose.const import LABEL_CONFIG_HASH
from compose.const import LABEL_CONTAINER_NUMBER
from compose.const import LABEL_ONE_OFF
from compose.const import LABEL_PROJECT
from compose.const import LABEL_SERVICE
from compose.const import LABEL_VERSION
from compose.container import Container
from compose.errors import OperationFailedError
from compose.project import OneOffFilter
from compose.service import ConvergencePlan
from compose.service import ConvergenceStrategy
from compose.service import NetworkMode
from compose.service import Service
from tests.integration.testcases import v2_1_only
from tests.integration.testcases import v2_2_only
from tests.integration.testcases import v2_only
from tests.integration.testcases import v3_only


def create_and_start_container(service, **override_options):
    container = service.create_container(**override_options)
    return service.start_container(container)


class ServiceTest(DockerClientTestCase):

    def test_containers(self):
        foo = self.create_service('foo')
        bar = self.create_service('bar')

        create_and_start_container(foo)

        self.assertEqual(len(foo.containers()), 1)
        self.assertEqual(foo.containers()[0].name, 'composetest_foo_1')
        self.assertEqual(len(bar.containers()), 0)

        create_and_start_container(bar)
        create_and_start_container(bar)

        self.assertEqual(len(foo.containers()), 1)
        self.assertEqual(len(bar.containers()), 2)

        names = [c.name for c in bar.containers()]
        self.assertIn('composetest_bar_1', names)
        self.assertIn('composetest_bar_2', names)

    def test_containers_one_off(self):
        db = self.create_service('db')
        container = db.create_container(one_off=True)
        self.assertEqual(db.containers(stopped=True), [])
        self.assertEqual(db.containers(one_off=OneOffFilter.only, stopped=True), [container])

    def test_project_is_added_to_container_name(self):
        service = self.create_service('web')
        create_and_start_container(service)
        self.assertEqual(service.containers()[0].name, 'composetest_web_1')

    def test_create_container_with_one_off(self):
        db = self.create_service('db')
        container = db.create_container(one_off=True)
        self.assertEqual(container.name, 'composetest_db_run_1')

    def test_create_container_with_one_off_when_existing_container_is_running(self):
        db = self.create_service('db')
        db.start()
        container = db.create_container(one_off=True)
        self.assertEqual(container.name, 'composetest_db_run_1')

    def test_create_container_with_unspecified_volume(self):
        service = self.create_service('db', volumes=[VolumeSpec.parse('/var/db')])
        container = service.create_container()
        service.start_container(container)
        assert container.get_mount('/var/db')

    def test_create_container_with_volume_driver(self):
        service = self.create_service('db', volume_driver='foodriver')
        container = service.create_container()
        service.start_container(container)
        self.assertEqual('foodriver', container.get('HostConfig.VolumeDriver'))

    def test_create_container_with_cpu_shares(self):
        service = self.create_service('db', cpu_shares=73)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(container.get('HostConfig.CpuShares'), 73)

    def test_create_container_with_cpu_quota(self):
        service = self.create_service('db', cpu_quota=40000)
        container = service.create_container()
        container.start()
        self.assertEqual(container.get('HostConfig.CpuQuota'), 40000)

    @v2_2_only()
    def test_create_container_with_cpu_count(self):
        self.require_api_version('1.25')
        service = self.create_service('db', cpu_count=2)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(container.get('HostConfig.CpuCount'), 2)

    @v2_2_only()
    @pytest.mark.skipif(not IS_WINDOWS_PLATFORM, reason='cpu_percent is not supported for Linux')
    def test_create_container_with_cpu_percent(self):
        self.require_api_version('1.25')
        service = self.create_service('db', cpu_percent=12)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(container.get('HostConfig.CpuPercent'), 12)

    @v2_2_only()
    def test_create_container_with_cpus(self):
        self.require_api_version('1.25')
        service = self.create_service('db', cpus=1)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(container.get('HostConfig.NanoCpus'), 1000000000)

    def test_create_container_with_shm_size(self):
        self.require_api_version('1.22')
        service = self.create_service('db', shm_size=67108864)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(container.get('HostConfig.ShmSize'), 67108864)

    def test_create_container_with_init_bool(self):
        self.require_api_version('1.25')
        service = self.create_service('db', init=True)
        container = service.create_container()
        service.start_container(container)
        assert container.get('HostConfig.Init') is True

    def test_create_container_with_init_path(self):
        self.require_api_version('1.25')
        docker_init_path = find_executable('docker-init')
        service = self.create_service('db', init=docker_init_path)
        container = service.create_container()
        service.start_container(container)
        assert container.get('HostConfig.InitPath') == docker_init_path

    @pytest.mark.xfail(True, reason='Some kernels/configs do not support pids_limit')
    def test_create_container_with_pids_limit(self):
        self.require_api_version('1.23')
        service = self.create_service('db', pids_limit=10)
        container = service.create_container()
        service.start_container(container)
        assert container.get('HostConfig.PidsLimit') == 10

    def test_create_container_with_extra_hosts_list(self):
        extra_hosts = ['somehost:162.242.195.82', 'otherhost:50.31.209.229']
        service = self.create_service('db', extra_hosts=extra_hosts)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(set(container.get('HostConfig.ExtraHosts')), set(extra_hosts))

    def test_create_container_with_extra_hosts_dicts(self):
        extra_hosts = {'somehost': '162.242.195.82', 'otherhost': '50.31.209.229'}
        extra_hosts_list = ['somehost:162.242.195.82', 'otherhost:50.31.209.229']
        service = self.create_service('db', extra_hosts=extra_hosts)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(set(container.get('HostConfig.ExtraHosts')), set(extra_hosts_list))

    def test_create_container_with_cpu_set(self):
        service = self.create_service('db', cpuset='0')
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(container.get('HostConfig.CpusetCpus'), '0')

    def test_create_container_with_read_only_root_fs(self):
        read_only = True
        service = self.create_service('db', read_only=read_only)
        container = service.create_container()
        service.start_container(container)
        assert container.get('HostConfig.ReadonlyRootfs') == read_only

    def test_create_container_with_security_opt(self):
        security_opt = ['label:disable']
        service = self.create_service('db', security_opt=security_opt)
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(set(container.get('HostConfig.SecurityOpt')), set(security_opt))

    def test_create_container_with_mac_address(self):
        service = self.create_service('db', mac_address='02:42:ac:11:65:43')
        container = service.create_container()
        service.start_container(container)
        self.assertEqual(container.inspect()['Config']['MacAddress'], '02:42:ac:11:65:43')

    def test_create_container_with_specified_volume(self):
        host_path = '/tmp/host-path'
        container_path = '/container-path'

        service = self.create_service(
            'db',
            volumes=[VolumeSpec(host_path, container_path, 'rw')])
        container = service.create_container()
        service.start_container(container)
        assert container.get_mount(container_path)

        # Match the last component ("host-path"), because boot2docker symlinks /tmp
        actual_host_path = container.get_mount(container_path)['Source']

        self.assertTrue(path.basename(actual_host_path) == path.basename(host_path),
                        msg=("Last component differs: %s, %s" % (actual_host_path, host_path)))

    def test_recreate_preserves_volume_with_trailing_slash(self):
        """When the Compose file specifies a trailing slash in the container path, make
        sure we copy the volume over when recreating.
        """
        service = self.create_service('data', volumes=[VolumeSpec.parse('/data/')])
        old_container = create_and_start_container(service)
        volume_path = old_container.get_mount('/data')['Source']

        new_container = service.recreate_container(old_container)
        self.assertEqual(new_container.get_mount('/data')['Source'], volume_path)

    def test_duplicate_volume_trailing_slash(self):
        """
        When an image specifies a volume, and the Compose file specifies a host path
        but adds a trailing slash, make sure that we don't create duplicate binds.
        """
        host_path = '/tmp/data'
        container_path = '/data'
        volumes = [VolumeSpec.parse('{}:{}/'.format(host_path, container_path))]

        tmp_container = self.client.create_container(
            'busybox', 'true',
            volumes={container_path: {}},
            labels={'com.docker.compose.test_image': 'true'},
        )
        image = self.client.commit(tmp_container)['Id']

        service = self.create_service('db', image=image, volumes=volumes)
        old_container = create_and_start_container(service)

        self.assertEqual(
            old_container.get('Config.Volumes'),
            {container_path: {}},
        )

        service = self.create_service('db', image=image, volumes=volumes)
        new_container = service.recreate_container(old_container)

        self.assertEqual(
            new_container.get('Config.Volumes'),
            {container_path: {}},
        )

        self.assertEqual(service.containers(stopped=False), [new_container])

    def test_create_container_with_volumes_from(self):
        volume_service = self.create_service('data')
        volume_container_1 = volume_service.create_container()
        volume_container_2 = Container.create(
            self.client,
            image='busybox:latest',
            command=["top"],
            labels={LABEL_PROJECT: 'composetest'},
        )
        host_service = self.create_service(
            'host',
            volumes_from=[
                VolumeFromSpec(volume_service, 'rw', 'service'),
                VolumeFromSpec(volume_container_2, 'rw', 'container')
            ]
        )
        host_container = host_service.create_container()
        host_service.start_container(host_container)
        self.assertIn(volume_container_1.id + ':rw',
                      host_container.get('HostConfig.VolumesFrom'))
        self.assertIn(volume_container_2.id + ':rw',
                      host_container.get('HostConfig.VolumesFrom'))

    def test_execute_convergence_plan_recreate(self):
        service = self.create_service(
            'db',
            environment={'FOO': '1'},
            volumes=[VolumeSpec.parse('/etc')],
            entrypoint=['top'],
            command=['-d', '1']
        )
        old_container = service.create_container()
        self.assertEqual(old_container.get('Config.Entrypoint'), ['top'])
        self.assertEqual(old_container.get('Config.Cmd'), ['-d', '1'])
        self.assertIn('FOO=1', old_container.get('Config.Env'))
        self.assertEqual(old_container.name, 'composetest_db_1')
        service.start_container(old_container)
        old_container.inspect()  # reload volume data
        volume_path = old_container.get_mount('/etc')['Source']

        num_containers_before = len(self.client.containers(all=True))

        service.options['environment']['FOO'] = '2'
        new_container, = service.execute_convergence_plan(
            ConvergencePlan('recreate', [old_container]))

        self.assertEqual(new_container.get('Config.Entrypoint'), ['top'])
        self.assertEqual(new_container.get('Config.Cmd'), ['-d', '1'])
        self.assertIn('FOO=2', new_container.get('Config.Env'))
        self.assertEqual(new_container.name, 'composetest_db_1')
        self.assertEqual(new_container.get_mount('/etc')['Source'], volume_path)
        self.assertIn(
            'affinity:container==%s' % old_container.id,
            new_container.get('Config.Env'))

        self.assertEqual(len(self.client.containers(all=True)), num_containers_before)
        self.assertNotEqual(old_container.id, new_container.id)
        self.assertRaises(APIError,
                          self.client.inspect_container,
                          old_container.id)

    def test_execute_convergence_plan_recreate_twice(self):
        service = self.create_service(
            'db',
            volumes=[VolumeSpec.parse('/etc')],
            entrypoint=['top'],
            command=['-d', '1'])

        orig_container = service.create_container()
        service.start_container(orig_container)

        orig_container.inspect()  # reload volume data
        volume_path = orig_container.get_mount('/etc')['Source']

        # Do this twice to reproduce the bug
        for _ in range(2):
            new_container, = service.execute_convergence_plan(
                ConvergencePlan('recreate', [orig_container]))

            assert new_container.get_mount('/etc')['Source'] == volume_path
            assert ('affinity:container==%s' % orig_container.id in
                    new_container.get('Config.Env'))

            orig_container = new_container

    def test_execute_convergence_plan_when_containers_are_stopped(self):
        service = self.create_service(
            'db',
            environment={'FOO': '1'},
            volumes=[VolumeSpec.parse('/var/db')],
            entrypoint=['top'],
            command=['-d', '1']
        )
        service.create_container()

        containers = service.containers(stopped=True)
        self.assertEqual(len(containers), 1)
        container, = containers
        self.assertFalse(container.is_running)

        service.execute_convergence_plan(ConvergencePlan('start', [container]))

        containers = service.containers()
        self.assertEqual(len(containers), 1)
        container.inspect()
        self.assertEqual(container, containers[0])
        self.assertTrue(container.is_running)

    def test_execute_convergence_plan_with_image_declared_volume(self):
        service = Service(
            project='composetest',
            name='db',
            client=self.client,
            build={'context': 'tests/fixtures/dockerfile-with-volume'},
        )

        old_container = create_and_start_container(service)
        self.assertEqual(
            [mount['Destination'] for mount in old_container.get('Mounts')], ['/data']
        )
        volume_path = old_container.get_mount('/data')['Source']

        new_container, = service.execute_convergence_plan(
            ConvergencePlan('recreate', [old_container]))

        self.assertEqual(
            [mount['Destination'] for mount in new_container.get('Mounts')],
            ['/data']
        )
        self.assertEqual(new_container.get_mount('/data')['Source'], volume_path)

    def test_execute_convergence_plan_when_image_volume_masks_config(self):
        service = self.create_service(
            'db',
            build={'context': 'tests/fixtures/dockerfile-with-volume'},
        )

        old_container = create_and_start_container(service)
        self.assertEqual(
            [mount['Destination'] for mount in old_container.get('Mounts')],
            ['/data']
        )
        volume_path = old_container.get_mount('/data')['Source']

        service.options['volumes'] = [VolumeSpec.parse('/tmp:/data')]

        with mock.patch('compose.service.log') as mock_log:
            new_container, = service.execute_convergence_plan(
                ConvergencePlan('recreate', [old_container]))

        mock_log.warn.assert_called_once_with(mock.ANY)
        _, args, kwargs = mock_log.warn.mock_calls[0]
        self.assertIn(
            "Service \"db\" is using volume \"/data\" from the previous container",
            args[0])

        self.assertEqual(
            [mount['Destination'] for mount in new_container.get('Mounts')],
            ['/data']
        )
        self.assertEqual(new_container.get_mount('/data')['Source'], volume_path)

    def test_execute_convergence_plan_when_host_volume_is_removed(self):
        host_path = '/tmp/host-path'
        service = self.create_service(
            'db',
            build={'context': 'tests/fixtures/dockerfile-with-volume'},
            volumes=[VolumeSpec(host_path, '/data', 'rw')])

        old_container = create_and_start_container(service)
        assert (
            [mount['Destination'] for mount in old_container.get('Mounts')] ==
            ['/data']
        )
        service.options['volumes'] = []

        with mock.patch('compose.service.log', autospec=True) as mock_log:
            new_container, = service.execute_convergence_plan(
                ConvergencePlan('recreate', [old_container]))

        assert not mock_log.warn.called
        assert (
            [mount['Destination'] for mount in new_container.get('Mounts')] ==
            ['/data']
        )
        assert new_container.get_mount('/data')['Source'] != host_path

    def test_execute_convergence_plan_without_start(self):
        service = self.create_service(
            'db',
            build={'context': 'tests/fixtures/dockerfile-with-volume'}
        )

        containers = service.execute_convergence_plan(ConvergencePlan('create', []), start=False)
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        containers = service.execute_convergence_plan(
            ConvergencePlan('recreate', containers),
            start=False)
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

        service.execute_convergence_plan(ConvergencePlan('start', containers), start=False)
        self.assertEqual(len(service.containers()), 0)
        self.assertEqual(len(service.containers(stopped=True)), 1)

    def test_start_container_passes_through_options(self):
        db = self.create_service('db')
        create_and_start_container(db, environment={'FOO': 'BAR'})
        self.assertEqual(db.containers()[0].environment['FOO'], 'BAR')

    def test_start_container_inherits_options_from_constructor(self):
        db = self.create_service('db', environment={'FOO': 'BAR'})
        create_and_start_container(db)
        self.assertEqual(db.containers()[0].environment['FOO'], 'BAR')

    def test_start_container_creates_links(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, None)])

        create_and_start_container(db)
        create_and_start_container(db)
        create_and_start_container(web)

        self.assertEqual(
            set(get_links(web.containers()[0])),
            set([
                'composetest_db_1', 'db_1',
                'composetest_db_2', 'db_2',
                'db'])
        )

    def test_start_container_creates_links_with_names(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[(db, 'custom_link_name')])

        create_and_start_container(db)
        create_and_start_container(db)
        create_and_start_container(web)

        self.assertEqual(
            set(get_links(web.containers()[0])),
            set([
                'composetest_db_1', 'db_1',
                'composetest_db_2', 'db_2',
                'custom_link_name'])
        )

    def test_start_container_with_external_links(self):
        db = self.create_service('db')
        web = self.create_service('web', external_links=['composetest_db_1',
                                                         'composetest_db_2',
                                                         'composetest_db_3:db_3'])

        for _ in range(3):
            create_and_start_container(db)
        create_and_start_container(web)

        self.assertEqual(
            set(get_links(web.containers()[0])),
            set([
                'composetest_db_1',
                'composetest_db_2',
                'db_3']),
        )

    def test_start_normal_container_does_not_create_links_to_its_own_service(self):
        db = self.create_service('db')

        create_and_start_container(db)
        create_and_start_container(db)

        c = create_and_start_container(db)
        self.assertEqual(set(get_links(c)), set([]))

    def test_start_one_off_container_creates_links_to_its_own_service(self):
        db = self.create_service('db')

        create_and_start_container(db)
        create_and_start_container(db)

        c = create_and_start_container(db, one_off=OneOffFilter.only)

        self.assertEqual(
            set(get_links(c)),
            set([
                'composetest_db_1', 'db_1',
                'composetest_db_2', 'db_2',
                'db'])
        )

    def test_start_container_builds_images(self):
        service = Service(
            name='test',
            client=self.client,
            build={'context': 'tests/fixtures/simple-dockerfile'},
            project='composetest',
        )
        container = create_and_start_container(service)
        container.wait()
        self.assertIn(b'success', container.logs())
        self.assertEqual(len(self.client.images(name='composetest_test')), 1)

    def test_start_container_uses_tagged_image_if_it_exists(self):
        self.check_build('tests/fixtures/simple-dockerfile', tag='composetest_test')
        service = Service(
            name='test',
            client=self.client,
            build={'context': 'this/does/not/exist/and/will/throw/error'},
            project='composetest',
        )
        container = create_and_start_container(service)
        container.wait()
        self.assertIn(b'success', container.logs())

    def test_start_container_creates_ports(self):
        service = self.create_service('web', ports=[8000])
        container = create_and_start_container(service).inspect()
        self.assertEqual(list(container['NetworkSettings']['Ports'].keys()), ['8000/tcp'])
        self.assertNotEqual(container['NetworkSettings']['Ports']['8000/tcp'][0]['HostPort'], '8000')

    def test_build(self):
        base_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base_dir)

        with open(os.path.join(base_dir, 'Dockerfile'), 'w') as f:
            f.write("FROM busybox\n")

        self.create_service('web', build={'context': base_dir}).build()
        assert self.client.inspect_image('composetest_web')

    def test_build_non_ascii_filename(self):
        base_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base_dir)

        with open(os.path.join(base_dir, 'Dockerfile'), 'w') as f:
            f.write("FROM busybox\n")

        with open(os.path.join(base_dir.encode('utf8'), b'foo\xE2bar'), 'w') as f:
            f.write("hello world\n")

        self.create_service('web', build={'context': text_type(base_dir)}).build()
        assert self.client.inspect_image('composetest_web')

    def test_build_with_image_name(self):
        base_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base_dir)

        with open(os.path.join(base_dir, 'Dockerfile'), 'w') as f:
            f.write("FROM busybox\n")

        image_name = 'examples/composetest:latest'
        self.addCleanup(self.client.remove_image, image_name)
        self.create_service('web', build={'context': base_dir}, image=image_name).build()
        assert self.client.inspect_image(image_name)

    def test_build_with_git_url(self):
        build_url = "https://github.com/dnephin/docker-build-from-url.git"
        service = self.create_service('buildwithurl', build={'context': build_url})
        self.addCleanup(self.client.remove_image, service.image_name)
        service.build()
        assert service.image()

    def test_build_with_build_args(self):
        base_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base_dir)

        with open(os.path.join(base_dir, 'Dockerfile'), 'w') as f:
            f.write("FROM busybox\n")
            f.write("ARG build_version\n")
            f.write("RUN echo ${build_version}\n")

        service = self.create_service('buildwithargs',
                                      build={'context': text_type(base_dir),
                                             'args': {"build_version": "1"}})
        service.build()
        assert service.image()
        assert "build_version=1" in service.image()['ContainerConfig']['Cmd']

    def test_build_with_build_args_override(self):
        base_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base_dir)

        with open(os.path.join(base_dir, 'Dockerfile'), 'w') as f:
            f.write("FROM busybox\n")
            f.write("ARG build_version\n")
            f.write("RUN echo ${build_version}\n")

        service = self.create_service('buildwithargs',
                                      build={'context': text_type(base_dir),
                                             'args': {"build_version": "1"}})
        service.build(build_args_override={'build_version': '2'})
        assert service.image()
        assert "build_version=2" in service.image()['ContainerConfig']['Cmd']

    def test_build_with_build_labels(self):
        base_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base_dir)

        with open(os.path.join(base_dir, 'Dockerfile'), 'w') as f:
            f.write('FROM busybox\n')

        service = self.create_service('buildlabels', build={
            'context': text_type(base_dir),
            'labels': {'com.docker.compose.test': 'true'}
        })
        service.build()
        assert service.image()
        assert service.image()['Config']['Labels']['com.docker.compose.test'] == 'true'

    def test_start_container_stays_unprivileged(self):
        service = self.create_service('web')
        container = create_and_start_container(service).inspect()
        self.assertEqual(container['HostConfig']['Privileged'], False)

    def test_start_container_becomes_privileged(self):
        service = self.create_service('web', privileged=True)
        container = create_and_start_container(service).inspect()
        self.assertEqual(container['HostConfig']['Privileged'], True)

    def test_expose_does_not_publish_ports(self):
        service = self.create_service('web', expose=["8000"])
        container = create_and_start_container(service).inspect()
        self.assertEqual(container['NetworkSettings']['Ports'], {'8000/tcp': None})

    def test_start_container_creates_port_with_explicit_protocol(self):
        service = self.create_service('web', ports=['8000/udp'])
        container = create_and_start_container(service).inspect()
        self.assertEqual(list(container['NetworkSettings']['Ports'].keys()), ['8000/udp'])

    def test_start_container_creates_fixed_external_ports(self):
        service = self.create_service('web', ports=['8000:8000'])
        container = create_and_start_container(service).inspect()
        self.assertIn('8000/tcp', container['NetworkSettings']['Ports'])
        self.assertEqual(container['NetworkSettings']['Ports']['8000/tcp'][0]['HostPort'], '8000')

    def test_start_container_creates_fixed_external_ports_when_it_is_different_to_internal_port(self):
        service = self.create_service('web', ports=['8001:8000'])
        container = create_and_start_container(service).inspect()
        self.assertIn('8000/tcp', container['NetworkSettings']['Ports'])
        self.assertEqual(container['NetworkSettings']['Ports']['8000/tcp'][0]['HostPort'], '8001')

    def test_port_with_explicit_interface(self):
        service = self.create_service('web', ports=[
            '127.0.0.1:8001:8000',
            '0.0.0.0:9001:9000/udp',
        ])
        container = create_and_start_container(service).inspect()
        self.assertEqual(container['NetworkSettings']['Ports'], {
            '8000/tcp': [
                {
                    'HostIp': '127.0.0.1',
                    'HostPort': '8001',
                },
            ],
            '9000/udp': [
                {
                    'HostIp': '0.0.0.0',
                    'HostPort': '9001',
                },
            ],
        })

    def test_create_with_image_id(self):
        # Get image id for the current busybox:latest
        pull_busybox(self.client)
        image_id = self.client.inspect_image('busybox:latest')['Id'][:12]
        service = self.create_service('foo', image=image_id)
        service.create_container()

    def test_scale(self):
        service = self.create_service('web')
        service.scale(1)
        self.assertEqual(len(service.containers()), 1)

        # Ensure containers don't have stdout or stdin connected
        container = service.containers()[0]
        config = container.inspect()['Config']
        self.assertFalse(config['AttachStderr'])
        self.assertFalse(config['AttachStdout'])
        self.assertFalse(config['AttachStdin'])

        service.scale(3)
        self.assertEqual(len(service.containers()), 3)
        service.scale(1)
        self.assertEqual(len(service.containers()), 1)
        service.scale(0)
        self.assertEqual(len(service.containers()), 0)

    def test_scale_with_stopped_containers(self):
        """
        Given there are some stopped containers and scale is called with a
        desired number that is the same as the number of stopped containers,
        test that those containers are restarted and not removed/recreated.
        """
        service = self.create_service('web')
        next_number = service._next_container_number()
        valid_numbers = [next_number, next_number + 1]
        service.create_container(number=next_number)
        service.create_container(number=next_number + 1)

        with mock.patch('sys.stderr', new_callable=StringIO) as mock_stderr:
            service.scale(2)
        for container in service.containers():
            self.assertTrue(container.is_running)
            self.assertTrue(container.number in valid_numbers)

        captured_output = mock_stderr.getvalue()
        self.assertNotIn('Creating', captured_output)
        self.assertIn('Starting', captured_output)

    def test_scale_with_stopped_containers_and_needing_creation(self):
        """
        Given there are some stopped containers and scale is called with a
        desired number that is greater than the number of stopped containers,
        test that those containers are restarted and required number are created.
        """
        service = self.create_service('web')
        next_number = service._next_container_number()
        service.create_container(number=next_number, quiet=True)

        for container in service.containers():
            self.assertFalse(container.is_running)

        with mock.patch('sys.stderr', new_callable=StringIO) as mock_stderr:
            service.scale(2)

        self.assertEqual(len(service.containers()), 2)
        for container in service.containers():
            self.assertTrue(container.is_running)

        captured_output = mock_stderr.getvalue()
        self.assertIn('Creating', captured_output)
        self.assertIn('Starting', captured_output)

    def test_scale_with_api_error(self):
        """Test that when scaling if the API returns an error, that error is handled
        and the remaining threads continue.
        """
        service = self.create_service('web')
        next_number = service._next_container_number()
        service.create_container(number=next_number, quiet=True)

        with mock.patch(
            'compose.container.Container.create',
            side_effect=APIError(
                message="testing",
                response={},
                explanation="Boom")):
            with mock.patch('sys.stderr', new_callable=StringIO) as mock_stderr:
                with pytest.raises(OperationFailedError):
                    service.scale(3)

        assert len(service.containers()) == 1
        assert service.containers()[0].is_running
        assert (
            "ERROR: for composetest_web_2  Cannot create container for service"
            " web: Boom" in mock_stderr.getvalue()
        )

    def test_scale_with_unexpected_exception(self):
        """Test that when scaling if the API returns an error, that is not of type
        APIError, that error is re-raised.
        """
        service = self.create_service('web')
        next_number = service._next_container_number()
        service.create_container(number=next_number, quiet=True)

        with mock.patch(
            'compose.container.Container.create',
            side_effect=ValueError("BOOM")
        ):
            with self.assertRaises(ValueError):
                service.scale(3)

        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

    @mock.patch('compose.service.log')
    def test_scale_with_desired_number_already_achieved(self, mock_log):
        """
        Test that calling scale with a desired number that is equal to the
        number of containers already running results in no change.
        """
        service = self.create_service('web')
        next_number = service._next_container_number()
        container = service.create_container(number=next_number, quiet=True)
        container.start()

        container.inspect()
        assert container.is_running
        assert len(service.containers()) == 1

        service.scale(1)
        assert len(service.containers()) == 1
        container.inspect()
        assert container.is_running

        captured_output = mock_log.info.call_args[0]
        assert 'Desired container number already achieved' in captured_output

    @mock.patch('compose.service.log')
    def test_scale_with_custom_container_name_outputs_warning(self, mock_log):
        """Test that calling scale on a service that has a custom container name
        results in warning output.
        """
        service = self.create_service('app', container_name='custom-container')
        self.assertEqual(service.custom_container_name, 'custom-container')

        with pytest.raises(OperationFailedError):
            service.scale(3)

        captured_output = mock_log.warn.call_args[0][0]

        self.assertEqual(len(service.containers()), 1)
        self.assertIn(
            "Remove the custom name to scale the service.",
            captured_output
        )

    def test_scale_sets_ports(self):
        service = self.create_service('web', ports=['8000'])
        service.scale(2)
        containers = service.containers()
        self.assertEqual(len(containers), 2)
        for container in containers:
            self.assertEqual(
                list(container.get('HostConfig.PortBindings')),
                ['8000/tcp'])

    def test_scale_with_immediate_exit(self):
        service = self.create_service('web', image='busybox', command='true')
        service.scale(2)
        assert len(service.containers(stopped=True)) == 2

    def test_network_mode_none(self):
        service = self.create_service('web', network_mode=NetworkMode('none'))
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.NetworkMode'), 'none')

    def test_network_mode_bridged(self):
        service = self.create_service('web', network_mode=NetworkMode('bridge'))
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.NetworkMode'), 'bridge')

    def test_network_mode_host(self):
        service = self.create_service('web', network_mode=NetworkMode('host'))
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.NetworkMode'), 'host')

    def test_pid_mode_none_defined(self):
        service = self.create_service('web', pid=None)
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.PidMode'), '')

    def test_pid_mode_host(self):
        service = self.create_service('web', pid='host')
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.PidMode'), 'host')

    @v2_1_only()
    def test_userns_mode_none_defined(self):
        service = self.create_service('web', userns_mode=None)
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.UsernsMode'), '')

    @v2_1_only()
    def test_userns_mode_host(self):
        service = self.create_service('web', userns_mode='host')
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.UsernsMode'), 'host')

    def test_dns_no_value(self):
        service = self.create_service('web')
        container = create_and_start_container(service)
        self.assertIsNone(container.get('HostConfig.Dns'))

    def test_dns_list(self):
        service = self.create_service('web', dns=['8.8.8.8', '9.9.9.9'])
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.Dns'), ['8.8.8.8', '9.9.9.9'])

    def test_mem_swappiness(self):
        service = self.create_service('web', mem_swappiness=11)
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.MemorySwappiness'), 11)

    def test_mem_reservation(self):
        service = self.create_service('web', mem_reservation='20m')
        container = create_and_start_container(service)
        assert container.get('HostConfig.MemoryReservation') == 20 * 1024 * 1024

    def test_restart_always_value(self):
        service = self.create_service('web', restart={'Name': 'always'})
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.RestartPolicy.Name'), 'always')

    def test_oom_score_adj_value(self):
        service = self.create_service('web', oom_score_adj=500)
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.OomScoreAdj'), 500)

    def test_group_add_value(self):
        service = self.create_service('web', group_add=["root", "1"])
        container = create_and_start_container(service)

        host_container_groupadd = container.get('HostConfig.GroupAdd')
        assert "root" in host_container_groupadd
        assert "1" in host_container_groupadd

    def test_dns_opt_value(self):
        service = self.create_service('web', dns_opt=["use-vc", "no-tld-query"])
        container = create_and_start_container(service)

        dns_opt = container.get('HostConfig.DnsOptions')
        assert 'use-vc' in dns_opt
        assert 'no-tld-query' in dns_opt

    def test_restart_on_failure_value(self):
        service = self.create_service('web', restart={
            'Name': 'on-failure',
            'MaximumRetryCount': 5
        })
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.RestartPolicy.Name'), 'on-failure')
        self.assertEqual(container.get('HostConfig.RestartPolicy.MaximumRetryCount'), 5)

    def test_cap_add_list(self):
        service = self.create_service('web', cap_add=['SYS_ADMIN', 'NET_ADMIN'])
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.CapAdd'), ['SYS_ADMIN', 'NET_ADMIN'])

    def test_cap_drop_list(self):
        service = self.create_service('web', cap_drop=['SYS_ADMIN', 'NET_ADMIN'])
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.CapDrop'), ['SYS_ADMIN', 'NET_ADMIN'])

    def test_dns_search(self):
        service = self.create_service('web', dns_search=['dc1.example.com', 'dc2.example.com'])
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.DnsSearch'), ['dc1.example.com', 'dc2.example.com'])

    @v2_only()
    def test_tmpfs(self):
        service = self.create_service('web', tmpfs=['/run'])
        container = create_and_start_container(service)
        self.assertEqual(container.get('HostConfig.Tmpfs'), {'/run': ''})

    def test_working_dir_param(self):
        service = self.create_service('container', working_dir='/working/dir/sample')
        container = service.create_container()
        self.assertEqual(container.get('Config.WorkingDir'), '/working/dir/sample')

    def test_split_env(self):
        service = self.create_service(
            'web',
            environment=['NORMAL=F1', 'CONTAINS_EQUALS=F=2', 'TRAILING_EQUALS='])
        env = create_and_start_container(service).environment
        for k, v in {'NORMAL': 'F1', 'CONTAINS_EQUALS': 'F=2', 'TRAILING_EQUALS': ''}.items():
            self.assertEqual(env[k], v)

    def test_env_from_file_combined_with_env(self):
        service = self.create_service(
            'web',
            environment=['ONE=1', 'TWO=2', 'THREE=3'],
            env_file=['tests/fixtures/env/one.env', 'tests/fixtures/env/two.env'])
        env = create_and_start_container(service).environment
        for k, v in {
            'ONE': '1',
            'TWO': '2',
            'THREE': '3',
            'FOO': 'baz',
            'DOO': 'dah'
        }.items():
            self.assertEqual(env[k], v)

    @v3_only()
    def test_build_with_cachefrom(self):
        base_dir = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, base_dir)

        with open(os.path.join(base_dir, 'Dockerfile'), 'w') as f:
            f.write("FROM busybox\n")

        service = self.create_service('cache_from',
                                      build={'context': base_dir,
                                             'cache_from': ['build1']})
        service.build()
        assert service.image()

    @mock.patch.dict(os.environ)
    def test_resolve_env(self):
        os.environ['FILE_DEF'] = 'E1'
        os.environ['FILE_DEF_EMPTY'] = 'E2'
        os.environ['ENV_DEF'] = 'E3'
        service = self.create_service(
            'web',
            environment={
                'FILE_DEF': 'F1',
                'FILE_DEF_EMPTY': '',
                'ENV_DEF': None,
                'NO_DEF': None
            }
        )
        env = create_and_start_container(service).environment
        for k, v in {
            'FILE_DEF': 'F1',
            'FILE_DEF_EMPTY': '',
            'ENV_DEF': 'E3',
            'NO_DEF': None
        }.items():
            self.assertEqual(env[k], v)

    def test_with_high_enough_api_version_we_get_default_network_mode(self):
        # TODO: remove this test once minimum docker version is 1.8.x
        with mock.patch.object(self.client, '_version', '1.20'):
            service = self.create_service('web')
            service_config = service._get_container_host_config({})
            self.assertEqual(service_config['NetworkMode'], 'default')

    def test_labels(self):
        labels_dict = {
            'com.example.description': "Accounting webapp",
            'com.example.department': "Finance",
            'com.example.label-with-empty-value': "",
        }

        compose_labels = {
            LABEL_CONTAINER_NUMBER: '1',
            LABEL_ONE_OFF: 'False',
            LABEL_PROJECT: 'composetest',
            LABEL_SERVICE: 'web',
            LABEL_VERSION: __version__,
        }
        expected = dict(labels_dict, **compose_labels)

        service = self.create_service('web', labels=labels_dict)
        labels = create_and_start_container(service).labels.items()
        for pair in expected.items():
            self.assertIn(pair, labels)

    def test_empty_labels(self):
        labels_dict = {'foo': '', 'bar': ''}
        service = self.create_service('web', labels=labels_dict)
        labels = create_and_start_container(service).labels.items()
        for name in labels_dict:
            self.assertIn((name, ''), labels)

    def test_stop_signal(self):
        stop_signal = 'SIGINT'
        service = self.create_service('web', stop_signal=stop_signal)
        container = create_and_start_container(service)
        self.assertEqual(container.stop_signal, stop_signal)

    def test_custom_container_name(self):
        service = self.create_service('web', container_name='my-web-container')
        self.assertEqual(service.custom_container_name, 'my-web-container')

        container = create_and_start_container(service)
        self.assertEqual(container.name, 'my-web-container')

        one_off_container = service.create_container(one_off=True)
        self.assertNotEqual(one_off_container.name, 'my-web-container')

    @pytest.mark.skipif(True, reason="Broken on 1.11.0 - 17.03.0")
    def test_log_drive_invalid(self):
        service = self.create_service('web', logging={'driver': 'xxx'})
        expected_error_msg = "logger: no log driver named 'xxx' is registered"

        with self.assertRaisesRegexp(APIError, expected_error_msg):
            create_and_start_container(service)

    def test_log_drive_empty_default_jsonfile(self):
        service = self.create_service('web')
        log_config = create_and_start_container(service).log_config

        self.assertEqual('json-file', log_config['Type'])
        self.assertFalse(log_config['Config'])

    def test_log_drive_none(self):
        service = self.create_service('web', logging={'driver': 'none'})
        log_config = create_and_start_container(service).log_config

        self.assertEqual('none', log_config['Type'])
        self.assertFalse(log_config['Config'])

    def test_devices(self):
        service = self.create_service('web', devices=["/dev/random:/dev/mapped-random"])
        device_config = create_and_start_container(service).get('HostConfig.Devices')

        device_dict = {
            'PathOnHost': '/dev/random',
            'CgroupPermissions': 'rwm',
            'PathInContainer': '/dev/mapped-random'
        }

        self.assertEqual(1, len(device_config))
        self.assertDictEqual(device_dict, device_config[0])

    def test_duplicate_containers(self):
        service = self.create_service('web')

        options = service._get_container_create_options({}, 1)
        original = Container.create(service.client, **options)

        self.assertEqual(set(service.containers(stopped=True)), set([original]))
        self.assertEqual(set(service.duplicate_containers()), set())

        options['name'] = 'temporary_container_name'
        duplicate = Container.create(service.client, **options)

        self.assertEqual(set(service.containers(stopped=True)), set([original, duplicate]))
        self.assertEqual(set(service.duplicate_containers()), set([duplicate]))


def converge(service, strategy=ConvergenceStrategy.changed):
    """Create a converge plan from a strategy and execute the plan."""
    plan = service.convergence_plan(strategy)
    return service.execute_convergence_plan(plan, timeout=1)


class ConfigHashTest(DockerClientTestCase):

    def test_no_config_hash_when_one_off(self):
        web = self.create_service('web')
        container = web.create_container(one_off=True)
        self.assertNotIn(LABEL_CONFIG_HASH, container.labels)

    def test_no_config_hash_when_overriding_options(self):
        web = self.create_service('web')
        container = web.create_container(environment={'FOO': '1'})
        self.assertNotIn(LABEL_CONFIG_HASH, container.labels)

    def test_config_hash_with_custom_labels(self):
        web = self.create_service('web', labels={'foo': '1'})
        container = converge(web)[0]
        self.assertIn(LABEL_CONFIG_HASH, container.labels)
        self.assertIn('foo', container.labels)

    def test_config_hash_sticks_around(self):
        web = self.create_service('web', command=["top"])
        container = converge(web)[0]
        self.assertIn(LABEL_CONFIG_HASH, container.labels)

        web = self.create_service('web', command=["top", "-d", "1"])
        container = converge(web)[0]
        self.assertIn(LABEL_CONFIG_HASH, container.labels)
