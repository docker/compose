from __future__ import unicode_literals
from __future__ import absolute_import
from .testcases import DockerClientTestCase
from mock import patch
from fig.cli.main import TopLevelCommand
from fig.packages.six import StringIO

class CLITestCase(DockerClientTestCase):
    def setUp(self):
        super(CLITestCase, self).setUp()
        self.command = TopLevelCommand()
        self.command.base_dir = 'tests/fixtures/simple-figfile'

    def tearDown(self):
        self.command.project.kill()
        self.command.project.remove_stopped()

    @patch('sys.stdout', new_callable=StringIO)
    def test_ps(self, mock_stdout):
        self.command.project.get_service('simple').create_container()
        self.command.dispatch(['ps'], None)
        self.assertIn('fig_simple_1', mock_stdout.getvalue())

    @patch('sys.stdout', new_callable=StringIO)
    def test_ps_default_figfile(self, mock_stdout):
        self.command.base_dir = 'tests/fixtures/multiple-figfiles'
        self.command.dispatch(['up', '-d'], None)
        self.command.dispatch(['ps'], None)

        output = mock_stdout.getvalue()
        self.assertIn('fig_simple_1', output)
        self.assertIn('fig_another_1', output)
        self.assertNotIn('fig_yetanother_1', output)

    @patch('sys.stdout', new_callable=StringIO)
    def test_ps_alternate_figfile(self, mock_stdout):
        self.command.base_dir = 'tests/fixtures/multiple-figfiles'
        self.command.dispatch(['-f', 'fig2.yml', 'up', '-d'], None)
        self.command.dispatch(['-f', 'fig2.yml', 'ps'], None)

        output = mock_stdout.getvalue()
        self.assertNotIn('fig_simple_1', output)
        self.assertNotIn('fig_another_1', output)
        self.assertIn('fig_yetanother_1', output)

    def test_rm(self):
        service = self.command.project.get_service('simple')
        service.create_container()
        service.kill()
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.command.dispatch(['rm', '--force'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)

    def test_scale(self):
        project = self.command.project

        self.command.scale({'SERVICE=NUM': ['simple=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)

        self.command.scale({'SERVICE=NUM': ['simple=3', 'another=2']})
        self.assertEqual(len(project.get_service('simple').containers()), 3)
        self.assertEqual(len(project.get_service('another').containers()), 2)

        self.command.scale({'SERVICE=NUM': ['simple=1', 'another=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.command.scale({'SERVICE=NUM': ['simple=1', 'another=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.command.scale({'SERVICE=NUM': ['simple=0', 'another=0']})
        self.assertEqual(len(project.get_service('simple').containers()), 0)
        self.assertEqual(len(project.get_service('another').containers()), 0)
