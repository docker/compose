"""
Integration tests which cover state convergence (aka smart recreate) performed
by `docker-compose up`.
"""
import copy
import os
import shutil
import tempfile

from docker.errors import ImageNotFound

from ..helpers import BUSYBOX_IMAGE_WITH_TAG
from .testcases import DockerClientTestCase
from .testcases import get_links
from .testcases import no_cluster
from compose.config import config
from compose.project import Project
from compose.service import ConvergenceStrategy


class ProjectTestCase(DockerClientTestCase):
    def run_up(self, cfg, **kwargs):
        kwargs.setdefault('timeout', 1)
        kwargs.setdefault('detached', True)

        project = self.make_project(cfg)
        project.up(**kwargs)
        return set(project.containers(stopped=True))

    def make_project(self, cfg):
        details = config.ConfigDetails(
            'working_dir',
            [config.ConfigFile(None, cfg)])
        return Project.from_config(
            name='composetest',
            client=self.client,
            config_data=config.load(details))


class BasicProjectTest(ProjectTestCase):
    def setUp(self):
        super().setUp()

        self.cfg = {
            'db': {'image': BUSYBOX_IMAGE_WITH_TAG, 'command': 'top'},
            'web': {'image': BUSYBOX_IMAGE_WITH_TAG, 'command': 'top'},
        }

    def test_no_change(self):
        old_containers = self.run_up(self.cfg)
        assert len(old_containers) == 2

        new_containers = self.run_up(self.cfg)
        assert len(new_containers) == 2

        assert old_containers == new_containers

    def test_partial_change(self):
        old_containers = self.run_up(self.cfg)
        old_db = [c for c in old_containers if c.name_without_project.startswith('db_')][0]
        old_web = [c for c in old_containers if c.name_without_project.startswith('web_')][0]

        self.cfg['web']['command'] = '/bin/true'

        new_containers = self.run_up(self.cfg)
        assert len(new_containers) == 2

        preserved = list(old_containers & new_containers)
        assert preserved == [old_db]

        removed = list(old_containers - new_containers)
        assert removed == [old_web]

        created = list(new_containers - old_containers)
        assert len(created) == 1
        assert created[0].name_without_project == old_web.name_without_project
        assert created[0].get('Config.Cmd') == ['/bin/true']

    def test_all_change(self):
        old_containers = self.run_up(self.cfg)
        assert len(old_containers) == 2

        self.cfg['web']['command'] = '/bin/true'
        self.cfg['db']['command'] = '/bin/true'

        new_containers = self.run_up(self.cfg)
        assert len(new_containers) == 2

        unchanged = old_containers & new_containers
        assert len(unchanged) == 0

        new = new_containers - old_containers
        assert len(new) == 2


class ProjectWithDependenciesTest(ProjectTestCase):
    def setUp(self):
        super().setUp()

        self.cfg = {
            'db': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'tail -f /dev/null',
            },
            'web': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'tail -f /dev/null',
                'links': ['db'],
            },
            'nginx': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'tail -f /dev/null',
                'links': ['web'],
            },
        }

    def test_up(self):
        containers = self.run_up(self.cfg)
        assert {c.service for c in containers} == {'db', 'web', 'nginx'}

    def test_change_leaf(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['nginx']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg)

        assert {c.service for c in new_containers - old_containers} == {'nginx'}

    def test_change_middle(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['web']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg)

        assert {c.service for c in new_containers - old_containers} == {'web'}

    def test_change_middle_always_recreate_deps(self):
        old_containers = self.run_up(self.cfg, always_recreate_deps=True)

        self.cfg['web']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg, always_recreate_deps=True)

        assert {c.service for c in new_containers - old_containers} == {'web', 'nginx'}

    def test_change_root(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg)

        assert {c.service for c in new_containers - old_containers} == {'db'}

    def test_change_root_always_recreate_deps(self):
        old_containers = self.run_up(self.cfg, always_recreate_deps=True)

        self.cfg['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(self.cfg, always_recreate_deps=True)

        assert {c.service for c in new_containers - old_containers} == {
            'db', 'web', 'nginx'
        }

    def test_change_root_no_recreate(self):
        old_containers = self.run_up(self.cfg)

        self.cfg['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(
            self.cfg,
            strategy=ConvergenceStrategy.never)

        assert new_containers - old_containers == set()

    def test_service_removed_while_down(self):
        next_cfg = {
            'web': {
                'image': BUSYBOX_IMAGE_WITH_TAG,
                'command': 'tail -f /dev/null',
            },
            'nginx': self.cfg['nginx'],
        }

        containers = self.run_up(self.cfg)
        assert len(containers) == 3

        project = self.make_project(self.cfg)
        project.stop(timeout=1)

        containers = self.run_up(next_cfg)
        assert len(containers) == 2

    def test_service_recreated_when_dependency_created(self):
        containers = self.run_up(self.cfg, service_names=['web'], start_deps=False)
        assert len(containers) == 1

        containers = self.run_up(self.cfg)
        assert len(containers) == 3

        web, = [c for c in containers if c.service == 'web']
        nginx, = [c for c in containers if c.service == 'nginx']
        db, = [c for c in containers if c.service == 'db']

        assert set(get_links(web)) == {
            'composetest_db_1',
            'db',
            'db_1',
        }
        assert set(get_links(nginx)) == {
            'composetest_web_1',
            'web',
            'web_1',
        }


class ProjectWithDependsOnDependenciesTest(ProjectTestCase):
    def setUp(self):
        super().setUp()

        self.cfg = {
            'version': '2',
            'services': {
                'db': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'tail -f /dev/null',
                },
                'web': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'tail -f /dev/null',
                    'depends_on': ['db'],
                },
                'nginx': {
                    'image': BUSYBOX_IMAGE_WITH_TAG,
                    'command': 'tail -f /dev/null',
                    'depends_on': ['web'],
                },
            }
        }

    def test_up(self):
        local_cfg = copy.deepcopy(self.cfg)
        containers = self.run_up(local_cfg)
        assert {c.service for c in containers} == {'db', 'web', 'nginx'}

    def test_change_leaf(self):
        local_cfg = copy.deepcopy(self.cfg)
        old_containers = self.run_up(local_cfg)

        local_cfg['services']['nginx']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(local_cfg)

        assert {c.service for c in new_containers - old_containers} == {'nginx'}

    def test_change_middle(self):
        local_cfg = copy.deepcopy(self.cfg)
        old_containers = self.run_up(local_cfg)

        local_cfg['services']['web']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(local_cfg)

        assert {c.service for c in new_containers - old_containers} == {'web'}

    def test_change_middle_always_recreate_deps(self):
        local_cfg = copy.deepcopy(self.cfg)
        old_containers = self.run_up(local_cfg, always_recreate_deps=True)

        local_cfg['services']['web']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(local_cfg, always_recreate_deps=True)

        assert {c.service for c in new_containers - old_containers} == {'web', 'nginx'}

    def test_change_root(self):
        local_cfg = copy.deepcopy(self.cfg)
        old_containers = self.run_up(local_cfg)

        local_cfg['services']['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(local_cfg)

        assert {c.service for c in new_containers - old_containers} == {'db'}

    def test_change_root_always_recreate_deps(self):
        local_cfg = copy.deepcopy(self.cfg)
        old_containers = self.run_up(local_cfg, always_recreate_deps=True)

        local_cfg['services']['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(local_cfg, always_recreate_deps=True)

        assert {c.service for c in new_containers - old_containers} == {'db', 'web', 'nginx'}

    def test_change_root_no_recreate(self):
        local_cfg = copy.deepcopy(self.cfg)
        old_containers = self.run_up(local_cfg)

        local_cfg['services']['db']['environment'] = {'NEW_VAR': '1'}
        new_containers = self.run_up(
            local_cfg,
            strategy=ConvergenceStrategy.never)

        assert new_containers - old_containers == set()

    def test_service_removed_while_down(self):
        local_cfg = copy.deepcopy(self.cfg)
        next_cfg = copy.deepcopy(self.cfg)
        del next_cfg['services']['db']
        del next_cfg['services']['web']['depends_on']

        containers = self.run_up(local_cfg)
        assert {c.service for c in containers} == {'db', 'web', 'nginx'}

        project = self.make_project(local_cfg)
        project.stop(timeout=1)

        next_containers = self.run_up(next_cfg)
        assert {c.service for c in next_containers} == {'web', 'nginx'}

    def test_service_removed_while_up(self):
        local_cfg = copy.deepcopy(self.cfg)
        containers = self.run_up(local_cfg)
        assert {c.service for c in containers} == {'db', 'web', 'nginx'}

        del local_cfg['services']['db']
        del local_cfg['services']['web']['depends_on']

        containers = self.run_up(local_cfg)
        assert {c.service for c in containers} == {'web', 'nginx'}

    def test_dependency_removed(self):
        local_cfg = copy.deepcopy(self.cfg)
        next_cfg = copy.deepcopy(self.cfg)
        del next_cfg['services']['nginx']['depends_on']

        containers = self.run_up(local_cfg, service_names=['nginx'])
        assert {c.service for c in containers} == {'db', 'web', 'nginx'}

        project = self.make_project(local_cfg)
        project.stop(timeout=1)

        next_containers = self.run_up(next_cfg, service_names=['nginx'])
        assert {c.service for c in next_containers if c.is_running} == {'nginx'}

    def test_dependency_added(self):
        local_cfg = copy.deepcopy(self.cfg)

        del local_cfg['services']['nginx']['depends_on']
        containers = self.run_up(local_cfg, service_names=['nginx'])
        assert {c.service for c in containers} == {'nginx'}

        local_cfg['services']['nginx']['depends_on'] = ['db']
        containers = self.run_up(local_cfg, service_names=['nginx'])
        assert {c.service for c in containers} == {'nginx', 'db'}


class ServiceStateTest(DockerClientTestCase):
    """Test cases for Service.convergence_plan."""

    def test_trigger_create(self):
        web = self.create_service('web')
        assert ('create', []) == web.convergence_plan()

    def test_trigger_noop(self):
        web = self.create_service('web')
        container = web.create_container()
        web.start()

        web = self.create_service('web')
        assert ('noop', [container]) == web.convergence_plan()

    def test_trigger_start(self):
        options = dict(command=["top"])

        web = self.create_service('web', **options)
        web.scale(2)

        containers = web.containers(stopped=True)
        containers[0].stop()
        containers[0].inspect()

        assert [c.is_running for c in containers] == [False, True]

        assert ('start', containers) == web.convergence_plan()

    def test_trigger_recreate_with_config_change(self):
        web = self.create_service('web', command=["top"])
        container = web.create_container()

        web = self.create_service('web', command=["top", "-d", "1"])
        assert ('recreate', [container]) == web.convergence_plan()

    def test_trigger_recreate_with_nonexistent_image_tag(self):
        web = self.create_service('web', image=BUSYBOX_IMAGE_WITH_TAG)
        container = web.create_container()

        web = self.create_service('web', image="nonexistent-image")
        assert ('recreate', [container]) == web.convergence_plan()

    def test_trigger_recreate_with_image_change(self):
        repo = 'composetest_myimage'
        tag = 'latest'
        image = '{}:{}'.format(repo, tag)

        def safe_remove_image(image):
            try:
                self.client.remove_image(image)
            except ImageNotFound:
                pass

        image_id = self.client.images(name='busybox')[0]['Id']
        self.client.tag(image_id, repository=repo, tag=tag)
        self.addCleanup(safe_remove_image, image)

        web = self.create_service('web', image=image)
        container = web.create_container()

        # update the image
        c = self.client.create_container(image, ['touch', '/hello.txt'], host_config={})

        # In the case of a cluster, there's a chance we pick up the old image when
        # calculating the new hash. To circumvent that, untag the old image first
        # See also: https://github.com/moby/moby/issues/26852
        self.client.remove_image(image, force=True)

        self.client.commit(c, repository=repo, tag=tag)
        self.client.remove_container(c)

        web = self.create_service('web', image=image)
        assert ('recreate', [container]) == web.convergence_plan()

    @no_cluster('Can not guarantee the build will be run on the same node the service is deployed')
    def test_trigger_recreate_with_build(self):
        context = tempfile.mkdtemp('test_trigger_recreate_with_build')
        self.addCleanup(shutil.rmtree, context)

        base_image = "FROM busybox\nLABEL com.docker.compose.test_image=true\n"
        dockerfile = os.path.join(context, 'Dockerfile')
        with open(dockerfile, mode="w") as dockerfile_fh:
            dockerfile_fh.write(base_image)

        web = self.create_service('web', build={'context': str(context)})
        container = web.create_container()

        with open(dockerfile, mode="w") as dockerfile_fh:
            dockerfile_fh.write(base_image + 'CMD echo hello world\n')
        web.build()

        web = self.create_service('web', build={'context': str(context)})
        assert ('recreate', [container]) == web.convergence_plan()

    def test_image_changed_to_build(self):
        context = tempfile.mkdtemp('test_image_changed_to_build')
        self.addCleanup(shutil.rmtree, context)
        with open(os.path.join(context, 'Dockerfile'), mode="w") as dockerfile:
            dockerfile.write("""
                FROM busybox
                LABEL com.docker.compose.test_image=true
            """)

        web = self.create_service('web', image='busybox')
        container = web.create_container()

        web = self.create_service('web', build={'context': str(context)})
        plan = web.convergence_plan()
        assert ('recreate', [container]) == plan
        containers = web.execute_convergence_plan(plan)
        assert len(containers) == 1
