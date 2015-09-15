"""
Integration tests which cover state convergence (aka smart recreate) performed
by `docker-compose up`.
"""
from __future__ import unicode_literals
import tempfile
import shutil
import os

from compose import config
from compose.project import Project
from compose.const import LABEL_CONFIG_HASH

from .testcases import DockerClientTestCase


class ProjectTestCase(DockerClientTestCase):
    def run_up(self, cfg, **kwargs):
        kwargs.setdefault('timeout', 1)

        project = self.make_project(cfg)
        project.up(**kwargs)
        return set(project.containers(stopped=True))

    def make_project(self, cfg):
        return Project.from_dicts(
            name='composetest',
            client=self.client,
            service_dicts=config.load(config.ConfigDetails(cfg, 'working_dir', None))
        )


class BasicProjectTest(ProjectTestCase):
    def setUp(self):
        super(BasicProjectTest, self).setUp()

        self.cfg = {
            'db': {'image': 'busybox:latest'},
            'web': {'image': 'busybox:latest'},
        }

    def test_no_change(self):
        old_containers = self.run_up(self.cfg)
        self.assertEqual(len(old_containers), 2)

        new_containers = self.run_up(self.cfg)
        self.assertEqual(len(new_containers), 2)

        self.assertEqual(old_containers, new_containers)

    def test_partial_change(self):
        old_containers = self.run_up(self.cfg)
        old_db = [c for c in old_containers if c.name_without_project == 'db_1'][0]
        old_web = [c for c in old_containers if c.name_without_project == 'web_1'][0]

        self.cfg['web']['command'] = '/bin/true'

        new_containers = self.run_up(self.cfg)
        self.assertEqual(len(new_containers), 2)

        preserved = list(old_containers & new_containers)
        self.assertEqual(preserved, [old_db])

        removed = list(old_containers - new_containers)
        self.assertEqual(removed, [old_web])

        created = list(new_containers - old_containers)
        self.assertEqual(len(created), 1)
        self.assertEqual(created[0].name_without_project, 'web_1')
        self.assertEqual(created[0].get('Config.Cmd'), ['/bin/true'])

    def test_all_change(self):
        old_containers = self.run_up(self.cfg)
        self.assertEqual(len(old_containers), 2)

        self.cfg['web']['command'] = '/bin/true'
        self.cfg['db']['command'] = '/bin/true'

        new_containers = self.run_up(self.cfg)
        self.assertEqual(len(new_containers), 2)

        unchanged = old_containers & new_containers
        self.assertEqual(len(unchanged), 0)

        new = new_containers - old_containers
        self.assertEqual(len(new), 2)


class ProjectWithDependenciesTest(ProjectTestCase):
    def setUp(self):
        super(ProjectWithDependenciesTest, self).setUp()

        self.cfg = {
            'db': {
                'image': 'busybox:latest',
                'command': 'tail -f /dev/null',
            },
            'web': {
                'image': 'busybox:latest',
                'command': 'tail -f /dev/null',
                'links': ['db'],
            },
            'nginx': {
                'image': 'busybox:latest',
                'command': 'tail -f /dev/null',
                'links': ['web'],
            },
        }

    def test_up(self):
        containers = self.run_up(self.cfg)
        self.assertEqual(
            set(c.name_without_project for c in containers),
            set(['db_1', 'web_1', 'nginx_1']),
        )

    def test_change_leaf(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['nginx']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg)

        self.assertEqual(
            set(c.name_without_project for c in new_containers - old_containers),
            set(['nginx_1']),
        )

    def test_change_middle(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['web']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg)

        self.assertEqual(
            set(c.name_without_project for c in new_containers - old_containers),
            set(['web_1', 'nginx_1']),
        )

    def test_change_root(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg)

        self.assertEqual(
            set(c.name_without_project for c in new_containers - old_containers),
            set(['db_1', 'web_1', 'nginx_1']),
        )

    def test_change_root_no_recreate(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg, allow_recreate=False)

        self.assertEqual(new_containers - old_containers, set())

    def test_service_removed_while_down(self):
        next_cfg = {
            'web': {
                'image': 'busybox:latest',
                'command': 'tail -f /dev/null',
            },
            'nginx': self.cfg['nginx'],
        }

        containers = self.run_up(self.cfg)
        self.assertEqual(len(containers), 3)

        project = self.make_project(self.cfg)
        project.stop(timeout=1)

        containers = self.run_up(next_cfg)
        self.assertEqual(len(containers), 2)


def converge(service,
             allow_recreate=True,
             force_recreate=False,
             do_build=True):
    """
    If a container for this service doesn't exist, create and start one. If there are
    any, stop them, create+start new ones, and remove the old containers.
    """
    plan = service.convergence_plan(
        allow_recreate=allow_recreate,
        force_recreate=force_recreate,
    )

    return service.execute_convergence_plan(
        plan,
        do_build=do_build,
        timeout=1,
    )


class ServiceStateTest(DockerClientTestCase):
    """Test cases for Service.convergence_plan."""

    def test_trigger_create(self):
        web = self.create_service('web')
        self.assertEqual(('create', []), web.convergence_plan())

    def test_trigger_noop(self):
        web = self.create_service('web')
        container = web.create_container()
        web.start()

        web = self.create_service('web')
        self.assertEqual(('noop', [container]), web.convergence_plan())

    def test_trigger_start(self):
        options = dict(command=["top"])

        web = self.create_service('web', **options)
        web.scale(2)

        containers = web.containers(stopped=True)
        containers[0].stop()
        containers[0].inspect()

        self.assertEqual([c.is_running for c in containers], [False, True])

        web = self.create_service('web', **options)
        self.assertEqual(
            ('start', containers[0:1]),
            web.convergence_plan(),
        )

    def test_trigger_recreate_with_config_change(self):
        web = self.create_service('web', command=["top"])
        container = web.create_container()

        web = self.create_service('web', command=["top", "-d", "1"])
        self.assertEqual(('recreate', [container]), web.convergence_plan())

    def test_trigger_recreate_with_nonexistent_image_tag(self):
        web = self.create_service('web', image="busybox:latest")
        container = web.create_container()

        web = self.create_service('web', image="nonexistent-image")
        self.assertEqual(('recreate', [container]), web.convergence_plan())

    def test_trigger_recreate_with_image_change(self):
        repo = 'composetest_myimage'
        tag = 'latest'
        image = '{}:{}'.format(repo, tag)

        image_id = self.client.images(name='busybox')[0]['Id']
        self.client.tag(image_id, repository=repo, tag=tag)

        try:
            web = self.create_service('web', image=image)
            container = web.create_container()

            # update the image
            c = self.client.create_container(image, ['touch', '/hello.txt'])
            self.client.commit(c, repository=repo, tag=tag)
            self.client.remove_container(c)

            web = self.create_service('web', image=image)
            self.assertEqual(('recreate', [container]), web.convergence_plan())

        finally:
            self.client.remove_image(image)

    def test_trigger_recreate_with_build(self):
        context = tempfile.mkdtemp()
        base_image = "FROM busybox\nLABEL com.docker.compose.test_image=true\n"

        try:
            dockerfile = os.path.join(context, 'Dockerfile')

            with open(dockerfile, 'w') as f:
                f.write(base_image)

            web = self.create_service('web', build=context)
            container = web.create_container()

            with open(dockerfile, 'w') as f:
                f.write(base_image + 'CMD echo hello world\n')
            web.build()

            web = self.create_service('web', build=context)
            self.assertEqual(('recreate', [container]), web.convergence_plan())
        finally:
            shutil.rmtree(context)


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
