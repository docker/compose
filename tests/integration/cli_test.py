from __future__ import absolute_import
from .testcases import DockerClientTestCase
from mock import patch
from fig.cli.main import TopLevelCommand
from fig.packages.six import StringIO
import sys

class CLITestCase(DockerClientTestCase):
    def setUp(self):
        super(CLITestCase, self).setUp()
        self.old_sys_exit = sys.exit
        sys.exit = lambda code=0: None
        self.command = TopLevelCommand()
        self.command.base_dir = 'tests/fixtures/simple-figfile'

    def tearDown(self):
        sys.exit = self.old_sys_exit
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

    def test_up(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.command.project.get_service('simple')
        another = self.command.project.get_service('another')
        self.assertEqual(len(service.containers()), 1)
        self.assertEqual(len(another.containers()), 1)

    def test_up_with_links(self):
        self.command.base_dir = 'tests/fixtures/links-figfile'
        self.command.dispatch(['up', '-d', 'web'], None)
        web = self.command.project.get_service('web')
        db = self.command.project.get_service('db')
        console = self.command.project.get_service('console')
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

    def test_up_with_no_deps(self):
        self.command.base_dir = 'tests/fixtures/links-figfile'
        self.command.dispatch(['up', '-d', '--no-deps', 'web'], None)
        web = self.command.project.get_service('web')
        db = self.command.project.get_service('db')
        console = self.command.project.get_service('console')
        self.assertEqual(len(web.containers()), 1)
        self.assertEqual(len(db.containers()), 0)
        self.assertEqual(len(console.containers()), 0)

    def test_up_with_recreate(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.command.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.command.dispatch(['up', '-d'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertNotEqual(old_ids, new_ids)

    def test_up_with_keep_old(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.command.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.command.dispatch(['up', '-d', '--no-recreate'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertEqual(old_ids, new_ids)


    @patch('dockerpty.start')
    def test_run_service_without_links(self, mock_stdout):
        self.command.base_dir = 'tests/fixtures/links-figfile'
        self.command.dispatch(['run', 'console', '/bin/true'], None)
        self.assertEqual(len(self.command.project.containers()), 0)

    @patch('dockerpty.start')
    def test_run_service_with_links(self, mock_stdout):
        self.command.base_dir = 'tests/fixtures/links-figfile'
        self.command.dispatch(['run', 'web', '/bin/true'], None)
        db = self.command.project.get_service('db')
        console = self.command.project.get_service('console')
        self.assertEqual(len(db.containers()), 1)
        self.assertEqual(len(console.containers()), 0)

    @patch('dockerpty.start')
    def test_run_with_no_deps(self, mock_stdout):
        mock_stdout.fileno = lambda: 1

        self.command.base_dir = 'tests/fixtures/links-figfile'
        self.command.dispatch(['run', '--no-deps', 'web', '/bin/true'], None)
        db = self.command.project.get_service('db')
        self.assertEqual(len(db.containers()), 0)

    @patch('dockerpty.start')
    def test_run_does_not_recreate_linked_containers(self, mock_stdout):
        mock_stdout.fileno = lambda: 1

        self.command.base_dir = 'tests/fixtures/links-figfile'
        self.command.dispatch(['up', '-d', 'db'], None)
        db = self.command.project.get_service('db')
        self.assertEqual(len(db.containers()), 1)

        old_ids = [c.id for c in db.containers()]

        self.command.dispatch(['run', 'web', '/bin/true'], None)
        self.assertEqual(len(db.containers()), 1)

        new_ids = [c.id for c in db.containers()]

        self.assertEqual(old_ids, new_ids)

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
