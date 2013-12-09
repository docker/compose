from plum.service import Service
from plum.service_collection import ServiceCollection
from .testcases import ServiceTestCase


class ServiceCollectionTest(ServiceTestCase):
    def test_from_dict(self):
        collection = ServiceCollection.from_dicts(None, [
            {
                'name': 'web',
                'image': 'ubuntu'
            },
            {
                'name': 'db',
                'image': 'ubuntu'
            }
        ])
        self.assertEqual(len(collection), 2)
        self.assertEqual(collection.get('web').name, 'web')
        self.assertEqual(collection.get('web').image, 'ubuntu')
        self.assertEqual(collection.get('db').name, 'db')
        self.assertEqual(collection.get('db').image, 'ubuntu')

    def test_from_dict_sorts_in_dependency_order(self):
        collection = ServiceCollection.from_dicts(None, [
            {
                'name': 'web',
                'image': 'ubuntu',
                'links': ['db'],
            },
            {
                'name': 'db',
                'image': 'ubuntu'
            }
        ])

        self.assertEqual(collection[0].name, 'db')
        self.assertEqual(collection[1].name, 'web')

    def test_get(self):
        web = self.create_service('web')
        collection = ServiceCollection([web])
        self.assertEqual(collection.get('web'), web)

    def test_start_stop(self):
        collection = ServiceCollection([
            self.create_service('web'),
            self.create_service('db'),
        ])

        collection.start()

        self.assertEqual(len(collection[0].containers), 1)
        self.assertEqual(len(collection[1].containers), 1)

        collection.stop()

        self.assertEqual(len(collection[0].containers), 0)
        self.assertEqual(len(collection[1].containers), 0)



