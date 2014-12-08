from __future__ import absolute_import
import sys

from six import StringIO
from mock import patch

from .testcases import DockerClientTestCase
from fig.cli.main import TopLevelCommand


class CLITestCase(DockerClientTestCase):
    def setUp(self):
        super(CLITestCase, self).setUp()
        self.old_sys_exit = sys.exit
        sys.exit = lambda code=0: None
        self.command = TopLevelCommand()
        self.command.base_dir = 'tests/fixtures/json-figfile'

    def tearDown(self):
        sys.exit = self.old_sys_exit
        self.project.kill()
        self.project.remove_stopped()

    @property
    def project(self):
        return self.command.get_project(self.command.get_config_path())

    @patch('sys.stdout', new_callable=StringIO)
    def test_ps(self, mock_stdout):
        self.project.get_service('simple').create_container()
        self.command.dispatch(['ps'], None)
        self.assertIn('jsonfigfile_simple_1', mock_stdout.getvalue())

    @patch('fig.service.log')
    def test_pull(self, mock_logging):
        self.command.dispatch(['pull'], None)
        mock_logging.info.assert_any_call('Pulling simple (busybox:latest)...')
        mock_logging.info.assert_any_call('Pulling another (busybox:latest)...')

    def test_up(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        another = self.project.get_service('another')
        self.assertEqual(len(service.containers()), 1)
        self.assertEqual(len(another.containers()), 1)

    def test_up_with_recreate(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.command.dispatch(['up', '-d'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertNotEqual(old_ids, new_ids)

    def test_up_with_keep_old(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)

        old_ids = [c.id for c in service.containers()]

        self.command.dispatch(['up', '-d', '--no-recreate'], None)
        self.assertEqual(len(service.containers()), 1)

        new_ids = [c.id for c in service.containers()]

        self.assertEqual(old_ids, new_ids)

    def test_rm(self):
        service = self.project.get_service('simple')
        service.create_container()
        service.kill()
        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.command.dispatch(['rm', '--force'], None)
        self.assertEqual(len(service.containers(stopped=True)), 0)

    def test_kill(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.command.dispatch(['kill'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_kill_signal_sigint(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.assertEqual(len(service.containers()), 1)
        self.assertTrue(service.containers()[0].is_running)

        self.command.dispatch(['kill', '-s', 'SIGINT'], None)

        self.assertEqual(len(service.containers()), 1)
        # The container is still running. It has been only interrupted
        self.assertTrue(service.containers()[0].is_running)

    def test_kill_interrupted_service(self):
        self.command.dispatch(['up', '-d'], None)
        service = self.project.get_service('simple')
        self.command.dispatch(['kill', '-s', 'SIGINT'], None)
        self.assertTrue(service.containers()[0].is_running)

        self.command.dispatch(['kill', '-s', 'SIGKILL'], None)

        self.assertEqual(len(service.containers(stopped=True)), 1)
        self.assertFalse(service.containers(stopped=True)[0].is_running)

    def test_restart(self):
        service = self.project.get_service('simple')
        container = service.create_container()
        service.start_container(container)
        started_at = container.dictionary['State']['StartedAt']
        self.command.dispatch(['restart'], None)
        container.inspect()
        self.assertNotEqual(
            container.dictionary['State']['FinishedAt'],
            '0001-01-01T00:00:00Z',
        )
        self.assertNotEqual(
            container.dictionary['State']['StartedAt'],
            started_at,
        )

    def test_scale(self):
        project = self.project

        self.command.scale(project, {'SERVICE=NUM': ['simple=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)

        self.command.scale(project, {'SERVICE=NUM': ['simple=3', 'another=2']})
        self.assertEqual(len(project.get_service('simple').containers()), 3)
        self.assertEqual(len(project.get_service('another').containers()), 2)

        self.command.scale(project, {'SERVICE=NUM': ['simple=1', 'another=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.command.scale(project, {'SERVICE=NUM': ['simple=1', 'another=1']})
        self.assertEqual(len(project.get_service('simple').containers()), 1)
        self.assertEqual(len(project.get_service('another').containers()), 1)

        self.command.scale(project, {'SERVICE=NUM': ['simple=0', 'another=0']})
        self.assertEqual(len(project.get_service('simple').containers()), 0)
        self.assertEqual(len(project.get_service('another').containers()), 0)
