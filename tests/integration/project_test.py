from __future__ import unicode_literals
from fig.project import Project, ConfigurationError
from fig.container import Container
from .testcases import DockerClientTestCase


class ProjectTest(DockerClientTestCase):
    def test_volumes_from_service(self):
        project = Project.from_config(
            name='figtest',
            config={
                'data': {
                    'image': 'busybox:latest',
                    'volumes': ['/var/data'],
                },
                'db': {
                    'image': 'busybox:latest',
                    'volumes_from': ['data'],
                },
            },
            client=self.client,
        )
        db = project.get_service('db')
        data = project.get_service('data')
        self.assertEqual(db.volumes_from, [data])

    def test_volumes_from_container(self):
        data_container = Container.create(
            self.client,
            image='busybox:latest',
            volumes=['/var/data'],
            name='figtest_data_container',
        )
        project = Project.from_config(
            name='figtest',
            config={
                'db': {
                    'image': 'busybox:latest',
                    'volumes_from': ['figtest_data_container'],
                },
            },
            client=self.client,
        )
        db = project.get_service('db')
        self.assertEqual(db.volumes_from, [data_container])

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
        db = self.create_service('db', volumes=['/var/db'])
        project = Project('figtest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        self.assertEqual(len(project.containers()), 1)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(web.containers()), 0)

        project.kill()
        project.remove_stopped()

    def test_project_up_recreates_containers(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=['/etc'])
        project = Project('figtest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        self.assertEqual(len(project.containers()), 1)
        old_db_id = project.containers()[0].id
        db_volume_path = project.containers()[0].get('Volumes./etc')

        project.up()
        self.assertEqual(len(project.containers()), 2)

        db_container = [c for c in project.containers() if 'db' in c.name][0]
        self.assertNotEqual(db_container.id, old_db_id)
        self.assertEqual(db_container.get('Volumes./etc'), db_volume_path)

        project.kill()
        project.remove_stopped()

    def test_project_up_with_no_recreate_running(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=['/var/db'])
        project = Project('figtest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        self.assertEqual(len(project.containers()), 1)
        old_db_id = project.containers()[0].id
        db_volume_path = project.containers()[0].inspect()['Volumes']['/var/db']

        project.up(recreate=False)
        self.assertEqual(len(project.containers()), 2)

        db_container = [c for c in project.containers() if 'db' in c.name][0]
        self.assertEqual(db_container.id, old_db_id)
        self.assertEqual(db_container.inspect()['Volumes']['/var/db'],
                         db_volume_path)

        project.kill()
        project.remove_stopped()

    def test_project_up_with_no_recreate_stopped(self):
        web = self.create_service('web')
        db = self.create_service('db', volumes=['/var/db'])
        project = Project('figtest', [web, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['db'])
        project.stop()

        old_containers = project.containers(stopped=True)

        self.assertEqual(len(old_containers), 1)
        old_db_id = old_containers[0].id
        db_volume_path = old_containers[0].inspect()['Volumes']['/var/db']

        project.up(recreate=False)

        new_containers = project.containers(stopped=True)
        self.assertEqual(len(new_containers), 2)

        db_container = [c for c in new_containers if 'db' in c.name][0]
        self.assertEqual(db_container.id, old_db_id)
        self.assertEqual(db_container.inspect()['Volumes']['/var/db'],
                         db_volume_path)

        project.kill()
        project.remove_stopped()

    def test_project_up_without_all_services(self):
        console = self.create_service('console')
        db = self.create_service('db')
        project = Project('figtest', [console, db], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up()
        self.assertEqual(len(project.containers()), 2)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 1)

        project.kill()
        project.remove_stopped()

    def test_project_up_starts_links(self):
        console = self.create_service('console')
        db = self.create_service('db', volumes=['/var/db'])
        web = self.create_service('web', links=[(db, 'db')])

        project = Project('figtest', [web, db, console], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['web'])
        self.assertEqual(len(project.containers()), 2)
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

        project.kill()
        project.remove_stopped()

    def test_project_up_with_no_deps(self):
        console = self.create_service('console')
        db = self.create_service('db', volumes=['/var/db'])
        web = self.create_service('web', links=[(db, 'db')])

        project = Project('figtest', [web, db, console], self.client)
        project.start()
        self.assertEqual(len(project.containers()), 0)

        project.up(['web'], start_links=False)
        self.assertEqual(len(project.containers()), 1)
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(console.containers()), 0)

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
