from unittest import TestCase
from docker import Client
from plum import Service


class NameTestCase(TestCase):
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


class ScalingTestCase(TestCase):
    def setUp(self):
        self.client = Client('http://127.0.0.1:4243')
        self.client.pull('ubuntu')

        for c in self.client.containers(all=True):
            self.client.kill(c['Id'])
            self.client.remove_container(c['Id'])

        self.service = Service(
            name="test",
            client=self.client,
            image="ubuntu",
            command=["/bin/sleep", "300"],
        )

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
