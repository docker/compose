from __future__ import absolute_import
from __future__ import unicode_literals

import pytest

from compose.config.errors import DependencyError
from compose.config.sort_services import sort_service_dicts
from compose.config.types import VolumeFromSpec
from tests import unittest


class SortServiceTest(unittest.TestCase):
    def test_sort_service_dicts_1(self):
        services = [
            {
                'links': ['redis'],
                'name': 'web'
            },
            {
                'name': 'grunt'
            },
            {
                'name': 'redis'
            }
        ]

        sorted_services = sort_service_dicts(services)
        self.assertEqual(len(sorted_services), 3)
        self.assertEqual(sorted_services[0]['name'], 'grunt')
        self.assertEqual(sorted_services[1]['name'], 'redis')
        self.assertEqual(sorted_services[2]['name'], 'web')

    def test_sort_service_dicts_2(self):
        services = [
            {
                'links': ['redis', 'postgres'],
                'name': 'web'
            },
            {
                'name': 'postgres',
                'links': ['redis']
            },
            {
                'name': 'redis'
            }
        ]

        sorted_services = sort_service_dicts(services)
        self.assertEqual(len(sorted_services), 3)
        self.assertEqual(sorted_services[0]['name'], 'redis')
        self.assertEqual(sorted_services[1]['name'], 'postgres')
        self.assertEqual(sorted_services[2]['name'], 'web')

    def test_sort_service_dicts_3(self):
        services = [
            {
                'name': 'child'
            },
            {
                'name': 'parent',
                'links': ['child']
            },
            {
                'links': ['parent'],
                'name': 'grandparent'
            },
        ]

        sorted_services = sort_service_dicts(services)
        self.assertEqual(len(sorted_services), 3)
        self.assertEqual(sorted_services[0]['name'], 'child')
        self.assertEqual(sorted_services[1]['name'], 'parent')
        self.assertEqual(sorted_services[2]['name'], 'grandparent')

    def test_sort_service_dicts_4(self):
        services = [
            {
                'name': 'child'
            },
            {
                'name': 'parent',
                'volumes_from': [VolumeFromSpec('child', 'rw', 'service')]
            },
            {
                'links': ['parent'],
                'name': 'grandparent'
            },
        ]

        sorted_services = sort_service_dicts(services)
        self.assertEqual(len(sorted_services), 3)
        self.assertEqual(sorted_services[0]['name'], 'child')
        self.assertEqual(sorted_services[1]['name'], 'parent')
        self.assertEqual(sorted_services[2]['name'], 'grandparent')

    def test_sort_service_dicts_5(self):
        services = [
            {
                'links': ['parent'],
                'name': 'grandparent'
            },
            {
                'name': 'parent',
                'net': 'container:child'
            },
            {
                'name': 'child'
            }
        ]

        sorted_services = sort_service_dicts(services)
        self.assertEqual(len(sorted_services), 3)
        self.assertEqual(sorted_services[0]['name'], 'child')
        self.assertEqual(sorted_services[1]['name'], 'parent')
        self.assertEqual(sorted_services[2]['name'], 'grandparent')

    def test_sort_service_dicts_6(self):
        services = [
            {
                'links': ['parent'],
                'name': 'grandparent'
            },
            {
                'name': 'parent',
                'volumes_from': [VolumeFromSpec('child', 'ro', 'service')]
            },
            {
                'name': 'child'
            }
        ]

        sorted_services = sort_service_dicts(services)
        self.assertEqual(len(sorted_services), 3)
        self.assertEqual(sorted_services[0]['name'], 'child')
        self.assertEqual(sorted_services[1]['name'], 'parent')
        self.assertEqual(sorted_services[2]['name'], 'grandparent')

    def test_sort_service_dicts_7(self):
        services = [
            {
                'net': 'container:three',
                'name': 'four'
            },
            {
                'links': ['two'],
                'name': 'three'
            },
            {
                'name': 'two',
                'volumes_from': [VolumeFromSpec('one', 'rw', 'service')]
            },
            {
                'name': 'one'
            }
        ]

        sorted_services = sort_service_dicts(services)
        self.assertEqual(len(sorted_services), 4)
        self.assertEqual(sorted_services[0]['name'], 'one')
        self.assertEqual(sorted_services[1]['name'], 'two')
        self.assertEqual(sorted_services[2]['name'], 'three')
        self.assertEqual(sorted_services[3]['name'], 'four')

    def test_sort_service_dicts_circular_imports(self):
        services = [
            {
                'links': ['redis'],
                'name': 'web'
            },
            {
                'name': 'redis',
                'links': ['web']
            },
        ]

        try:
            sort_service_dicts(services)
        except DependencyError as e:
            self.assertIn('redis', e.msg)
            self.assertIn('web', e.msg)
        else:
            self.fail('Should have thrown an DependencyError')

    def test_sort_service_dicts_circular_imports_2(self):
        services = [
            {
                'links': ['postgres', 'redis'],
                'name': 'web'
            },
            {
                'name': 'redis',
                'links': ['web']
            },
            {
                'name': 'postgres'
            }
        ]

        try:
            sort_service_dicts(services)
        except DependencyError as e:
            self.assertIn('redis', e.msg)
            self.assertIn('web', e.msg)
        else:
            self.fail('Should have thrown an DependencyError')

    def test_sort_service_dicts_circular_imports_3(self):
        services = [
            {
                'links': ['b'],
                'name': 'a'
            },
            {
                'name': 'b',
                'links': ['c']
            },
            {
                'name': 'c',
                'links': ['a']
            }
        ]

        try:
            sort_service_dicts(services)
        except DependencyError as e:
            self.assertIn('a', e.msg)
            self.assertIn('b', e.msg)
        else:
            self.fail('Should have thrown an DependencyError')

    def test_sort_service_dicts_self_imports(self):
        services = [
            {
                'links': ['web'],
                'name': 'web'
            },
        ]

        try:
            sort_service_dicts(services)
        except DependencyError as e:
            self.assertIn('web', e.msg)
        else:
            self.fail('Should have thrown an DependencyError')

    def test_sort_service_dicts_depends_on_self(self):
        services = [
            {
                'depends_on': ['web'],
                'name': 'web'
            },
        ]

        with pytest.raises(DependencyError) as exc:
            sort_service_dicts(services)
        assert 'A service can not depend on itself: web' in exc.exconly()
