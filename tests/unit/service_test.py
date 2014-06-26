from __future__ import unicode_literals
from __future__ import absolute_import
from .. import unittest
from fig import Service
from fig.service import ConfigError
from fig.container import Container

class ServiceTest(unittest.TestCase):
    def test_name_validations(self):
        self.assertRaises(ConfigError, lambda: Service(name=''))

        self.assertRaises(ConfigError, lambda: Service(name=' '))
        self.assertRaises(ConfigError, lambda: Service(name='/'))
        self.assertRaises(ConfigError, lambda: Service(name='!'))
        self.assertRaises(ConfigError, lambda: Service(name='\xe2'))
        self.assertRaises(ConfigError, lambda: Service(name='_'))
        self.assertRaises(ConfigError, lambda: Service(name='____'))
        self.assertRaises(ConfigError, lambda: Service(name='foo_bar'))
        self.assertRaises(ConfigError, lambda: Service(name='__foo_bar__'))

        Service('a')
        Service('foo')

    def test_project_validation(self):
        self.assertRaises(ConfigError, lambda: Service(name='foo', project='_'))
        Service(name='foo', project='bar')

    def test_config_validation(self):
        self.assertRaises(ConfigError, lambda: Service(name='foo', port=['8000']))
        Service(name='foo', ports=['8000'])

    def test_start_with_portbindings(self):
        s = Service(name='foo')
        container = Container(None, {})
        # testdata: (configsetting, {internal:external})
        testdata = (
          (["10:20/udp"],{"20/udp":"10"}),
          (["10:20"],{"20":"10"}),
          (["192.168.0.1:10:20"],{"20":("192.168.0.1","10")}),
          (["172.17.42.1:53:53/udp"],{"53/udp":("172.17.42.1","53")}),
        )
        for (conf,exp) in testdata:
          container.start = lambda **kwargs : self.assertEqual(kwargs['port_bindings'],exp)
          s.start_container(container, None, **{"ports":conf})
        
