from __future__ import unicode_literals
from __future__ import absolute_import
import os

from .. import unittest
from fig.cli import templates
from fig.cli.main import TopLevelCommand


class TemplatesTestCase(unittest.TestCase):
    def test_get_template_values_for_str(self):
        path, service = templates.get_template_values('name', 'template.yml')
        self.assertEquals('name', service)
        self.assertEquals('template.yml', path)

    def test_get_template_values_for_dict(self):
        path, service = templates.get_template_values('name', {
            'service': 'another',
            'path': 'hi.yml',
        })
        self.assertEquals('another', service)
        self.assertEquals('hi.yml', path)

    def test_template_load(self):
        cwd = os.getcwd()

        try:
            os.chdir('tests/fixtures/template-figfiles')
            command = TopLevelCommand()
            config = command.get_config('fig.yml')
            self.assertEquals({
                'image': 'busybox:latest',
                'command': '/bin/sleep 5',
            }, config.get('simple'))

            self.assertEquals({
                'image': 'busybox:latest',
                'command': '/bin/sleep 300',
                'ports': ['3000'],
            }, config.get('another'))
        finally:
            os.chdir(cwd)
