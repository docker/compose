from unittest import TestCase
from docker import Client
from plum import Service


client = Client('http://127.0.0.1:4243')
client.pull('ubuntu')


class ServiceTestCase(TestCase):
    def setUp(self):
        for c in client.containers(all=True):
            client.kill(c['Id'])
            client.remove_container(c['Id'])

    def create_service(self, name):
        return Service(
            name=name,
            client=client,
            image="ubuntu",
            command=["/bin/sleep", "300"],
        )


class NameTestCase(ServiceTestCase):
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


class ContainersTestCase(ServiceTestCase):
    def test_containers(self):
        foo = self.create_service('foo')
        bar = self.create_service('bar')

        foo.start()

        self.assertEqual(len(foo.containers), 1)
        self.assertEqual(len(bar.containers), 0)

        bar.scale(2)

        self.assertEqual(len(foo.containers), 1)
        self.assertEqual(len(bar.containers), 2)


class ScalingTestCase(ServiceTestCase):
    def setUp(self):
        super(ServiceTestCase, self).setUp()
        self.service = self.create_service("scaling_test")

    def test_up_scale_down(self):
        self.assertEqual(len(self.service.containers), 0)

        self.service.start()
        self.assertEqual(len(self.service.containers), 1)

        self.service.start()
        self.assertEqual(len(self.service.containers), 1)

        self.service.scale(2)
        self.assertEqual(len(self.service.containers), 2)

        self.service.scale(1)
        self.assertEqual(len(self.service.containers), 1)

        self.service.stop()
        self.assertEqual(len(self.service.containers), 0)

        self.service.stop()
        self.assertEqual(len(self.service.containers), 0)
