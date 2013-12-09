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
        web = [s for s in collection if s.name == 'web'][0]
        self.assertEqual(web.name, 'web')
        self.assertEqual(web.image, 'ubuntu')
        db = [s for s in collection if s.name == 'db'][0]
        self.assertEqual(db.name, 'db')
        self.assertEqual(db.image, 'ubuntu')

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
