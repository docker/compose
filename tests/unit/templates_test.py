from __future__ import unicode_literals
from __future__ import absolute_import
import os

from .. import unittest
from fig.cli.errors import FigFileNotFound
from fig.service import ConfigError
from fig.cli.main import TopLevelCommand
from fig.cli.templates import TemplateLoader


class TemplatesTestCase(unittest.TestCase):
    def test_get_template_values_for_str(self):
        path, service = TemplateLoader.get_template_values('template.yml',
            'name', 'self.yml')
        self.assertEquals('name', service)
        self.assertEquals('template.yml', path)

    def test_get_template_values_for_dict(self):
        path, service = TemplateLoader.get_template_values({
            'service': 'another',
            'path': 'hi.yml',
        }, 'name', 'self.yml')
        self.assertEquals('another', service)
        self.assertEquals('hi.yml', path)

    def test_get_template_values_for_dict_empty_path(self):
        path, service = TemplateLoader.get_template_values({
            'service': 'another',
        }, 'name', 'self.yml')
        self.assertEquals('another', service)
        self.assertEquals('self.yml', path)

    def test_get_template_values_for_dict_empty_service(self):
        path, service = TemplateLoader.get_template_values({
            'path': 'hi.yml',
        }, 'name', 'self.yml')
        self.assertEquals('name', service)
        self.assertEquals('hi.yml', path)

    def test_get_template_values_for_dict_totally_empty(self):
        with self.assertRaises(ConfigError):
            TemplateLoader.get_template_values(None, 'hello', 'path')
        with self.assertRaises(ConfigError):
            TemplateLoader.get_template_values({}, 'hello', 'path')

    def test_template_str(self):
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

    def test_template_self_ref(self):
        cwd = os.getcwd()

        try:
            os.chdir('tests/fixtures/template-figfiles')
            command = TopLevelCommand()
            config = command.get_config('fig-self-referencing.yml')

            self.assertEquals({
                'image': 'busybox:latest',
                'command': '/bin/bash',
            }, config.get('myservice'))
        finally:
            os.chdir(cwd)

    def test_template_missing_template_file(self):
        cwd = os.getcwd()

        try:
            os.chdir('tests/fixtures/template-figfiles')
            command = TopLevelCommand()
            with self.assertRaises(FigFileNotFound):
                command.get_config('fig-missing-template-file.yml')

        finally:
            os.chdir(cwd)

    def test_template_bad_self_reference(self):
        cwd = os.getcwd()

        try:
            os.chdir('tests/fixtures/template-figfiles')
            command = TopLevelCommand()
            with self.assertRaises(ConfigError):
                command.get_config('fig-missing-self-reference.yml')

        finally:
            os.chdir(cwd)

    def test_template_bad_external_reference(self):
        cwd = os.getcwd()

        try:
            os.chdir('tests/fixtures/template-figfiles')
            command = TopLevelCommand()
            with self.assertRaises(ConfigError):
                command.get_config('fig-missing-external-reference.yml')

        finally:
            os.chdir(cwd)
