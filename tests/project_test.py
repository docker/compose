from plum.project import Project
from plum.service import Service
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

    def test_start_stop(self):
        project = Project('test', [
            self.create_service('web'),
            self.create_service('db'),
        ], self.client)

        project.start()

        self.assertEqual(len(project.get_service('web').containers()), 1)
        self.assertEqual(len(project.get_service('db').containers()), 1)

        project.stop()

        self.assertEqual(len(project.get_service('web').containers()), 0)
        self.assertEqual(len(project.get_service('db').containers()), 0)
