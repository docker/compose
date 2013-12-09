from unittest import TestCase
from docker import Client
from plum import Service


class ServiceTestCase(TestCase):
    def setUp(self):
        self.client = Client('http://127.0.0.1:4243')
        self.client.pull('ubuntu')

        for c in self.client.containers():
            self.client.kill(c['Id'])

        self.service = Service(
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
