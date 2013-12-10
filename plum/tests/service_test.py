from plum import Service
from .testcases import DockerClientTestCase


class NameTestCase(DockerClientTestCase):
    def test_name_validations(self):
        self.assertRaises(ValueError, lambda: Service(name=''))

        self.assertRaises(ValueError, lambda: Service(name=' '))
        self.assertRaises(ValueError, lambda: Service(name='/'))
        self.assertRaises(ValueError, lambda: Service(name='!'))
        self.assertRaises(ValueError, lambda: Service(name='\xe2'))

        Service('a')
        Service('foo')
        Service('foo_bar')
        Service('__foo_bar__')
        Service('_')
        Service('_____')

    def test_containers(self):
        foo = self.create_service('foo')
        bar = self.create_service('bar')

        foo.start()

        self.assertEqual(len(foo.containers), 1)
        self.assertEqual(foo.containers[0]['Names'], ['/foo_1'])
        self.assertEqual(len(bar.containers), 0)

        bar.scale(2)

        self.assertEqual(len(foo.containers), 1)
        self.assertEqual(len(bar.containers), 2)

        names = [c['Names'] for c in bar.containers]
        self.assertIn(['/bar_1'], names)
        self.assertIn(['/bar_2'], names)

    def test_up_scale_down(self):
        service = self.create_service('scaling_test')
        self.assertEqual(len(service.containers), 0)

        service.start()
        self.assertEqual(len(service.containers), 1)

        service.start()
        self.assertEqual(len(service.containers), 1)

        service.scale(2)
        self.assertEqual(len(service.containers), 2)

        service.scale(1)
        self.assertEqual(len(service.containers), 1)

        service.stop()
        self.assertEqual(len(service.containers), 0)

        service.stop()
        self.assertEqual(len(service.containers), 0)

    def test_links_are_created_when_starting(self):
        db = self.create_service('db')
        web = self.create_service('web', links=[db])
        db.start()
        web.start()
        self.assertIn('/web_1/db_1', db.containers[0]['Names'])
        db.stop()
        web.stop()


