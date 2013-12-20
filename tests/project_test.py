from fig.project import Project
from fig.service import Service
from .testcases import DockerClientTestCase


class ProjectTest(DockerClientTestCase):
    def test_from_dict(self):
        project = Project.from_dicts('test', [
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
        project = Project.from_dicts('test', [
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

    def test_get_service(self):
        web = self.create_service('web')
        project = Project('test', [web], self.client)
        self.assertEqual(project.get_service('web'), web)

    def test_create_containers(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('test', [web, db], self.client)

        unstarted = project.create_containers(service_names=['web'])
        self.assertEqual(len(unstarted), 1)
        self.assertEqual(unstarted[0][0], web)
        self.assertEqual(len(web.containers(stopped=True)), 1)
        self.assertEqual(len(db.containers(stopped=True)), 0)

        unstarted = project.create_containers()
        self.assertEqual(len(unstarted), 2)
        self.assertEqual(unstarted[0][0], web)
        self.assertEqual(unstarted[1][0], db)
        self.assertEqual(len(web.containers(stopped=True)), 2)
        self.assertEqual(len(db.containers(stopped=True)), 1)

    def test_up(self):
        web = self.create_service('web')
        db = self.create_service('db')
        other = self.create_service('other')
        project = Project('test', [web, db, other], self.client)

        web.create_container()

        self.assertEqual(len(web.containers()), 0)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(web.containers(stopped=True)), 1)
        self.assertEqual(len(db.containers(stopped=True)), 0)

        unstarted = project.create_containers(service_names=['web', 'db'])
        self.assertEqual(len(unstarted), 2)
        self.assertEqual(unstarted[0][0], web)
        self.assertEqual(unstarted[1][0], db)

        self.assertEqual(len(web.containers()), 0)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(web.containers(stopped=True)), 2)
        self.assertEqual(len(db.containers(stopped=True)), 1)

        project.kill_and_remove(unstarted)

        self.assertEqual(len(web.containers()), 0)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(web.containers(stopped=True)), 1)
        self.assertEqual(len(db.containers(stopped=True)), 0)

    def test_start_stop_kill_remove(self):
        web = self.create_service('web')
        db = self.create_service('db')
        project = Project('test', [web, db], self.client)

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
