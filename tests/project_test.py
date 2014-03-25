from __future__ import unicode_literals
from fig.project import Project, ConfigurationError
from .testcases import DockerClientTestCase


class ProjectTest(DockerClientTestCase):
    def test_from_dict(self):
        project = Project.from_dicts('figtest', [
            {
                'name': 'web',
                'image': 'ubuntu'
            },
            {
                'name': 'db',
                'image': 'ubuntu'
            }
        ], self.client)
        self.assertEqual(len(project.services), 2)
        self.assertEqual(project.get_service('web').name, 'web')
        self.assertEqual(project.get_service('web').options['image'], 'ubuntu')
        self.assertEqual(project.get_service('db').name, 'db')
        self.assertEqual(project.get_service('db').options['image'], 'ubuntu')

    def test_from_dict_sorts_in_dependency_order(self):
        project = Project.from_dicts('figtest', [
            {
                'name': 'web',
                'image': 'ubuntu',
                'links': ['db'],
            },
            {
                'name': 'db',
                'image': 'ubuntu'
            }
        ], self.client)

        self.assertEqual(project.services[0].name, 'db')
        self.assertEqual(project.services[1].name, 'web')

    def test_from_config(self):
        project = Project.from_config('figtest', {
            'web': {
                'image': 'ubuntu',
            },
            'db': {
                'image': 'ubuntu',
            },
        }, self.client)
        self.assertEqual(len(project.services), 2)
        self.assertEqual(project.get_service('web').name, 'web')
        self.assertEqual(project.get_service('web').options['image'], 'ubuntu')
        self.assertEqual(project.get_service('db').name, 'db')
        self.assertEqual(project.get_service('db').options['image'], 'ubuntu')

    def test_from_config_throws_error_when_not_dict(self):
        with self.assertRaises(ConfigurationError):
            project = Project.from_config('figtest', {
                'web': 'ubuntu',
            }, self.client)

    def test_get_service(self):
        web = self.create_service('web')
        project = Project('test', [web], self.client)
        self.assertEqual(project.get_service('web'), web)

    def test_recreate_containers(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('test', [web, db], self.client)

        old_web_container = web.create_container()
        self.assertEqual(len(web.containers(stopped=True)), 1)
        self.assertEqual(len(db.containers(stopped=True)), 0)

        (old, new) = project.recreate_containers()
        self.assertEqual(len(old), 1)
        self.assertEqual(old[0][0], web)
        self.assertEqual(len(new), 2)
        self.assertEqual(new[0][0], web)
        self.assertEqual(new[1][0], db)

        self.assertEqual(len(web.containers(stopped=True)), 1)
        self.assertEqual(len(db.containers(stopped=True)), 1)

        # remove intermediate containers
        for (service, container) in old:
            container.remove()

    def test_start_stop_kill_remove(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('figtest', [web, db], self.client)

        project.start()

        self.assertEqual(len(web.containers()), 0)
        self.assertEqual(len(db.containers()), 0)

        web_container_1 = web.create_container()
        web_container_2 = web.create_container()
        db_container = db.create_container()

        project.start(service_names=['web'])
        self.assertEqual(set(c.name for c in project.containers()), set([web_container_1.name, web_container_2.name]))

        project.start()
        self.assertEqual(set(c.name for c in project.containers()), set([web_container_1.name, web_container_2.name, db_container.name]))

        project.stop(service_names=['web'], timeout=1)
        self.assertEqual(set(c.name for c in project.containers()), set([db_container.name]))

        project.kill(service_names=['db'])
        self.assertEqual(len(project.containers()), 0)
        self.assertEqual(len(project.containers(stopped=True)), 3)

        project.remove_stopped(service_names=['web'])
        self.assertEqual(len(project.containers(stopped=True)), 1)

        project.remove_stopped()
        self.assertEqual(len(project.containers(stopped=True)), 0)

    def test_project_up(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('figtest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)
        project.up()
        self.assertEqual(len(project.containers()), 2)
        project.kill()
        project.remove_stopped()

    def test_unscale_after_restart(self):
        web = self.create_service('web')
        project = Project('figtest', [web], self.client)

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
        project.kill()
        project.remove_stopped()
