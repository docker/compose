from __future__ import absolute_import
from __future__ import unicode_literals

import tempfile

from ddt import data
from ddt import ddt

from .. import mock
from ..acceptance.cli_test import dispatch
from compose.cli.command import get_project
from compose.cli.command import project_from_options
from compose.config.environment import Environment
from tests.integration.testcases import DockerClientTestCase


@ddt
class EnvironmentTest(DockerClientTestCase):
    @classmethod
    def setUpClass(cls):
        super(EnvironmentTest, cls).setUpClass()
        cls.compose_file = tempfile.NamedTemporaryFile(mode='w+b')
        cls.compose_file.write(bytes("""version: '3.2'
services:
  svc:
    image: busybox:1.31.0-uclibc
    environment:
      TEST_VARIABLE: ${TEST_VARIABLE}""", encoding='utf-8'))
        cls.compose_file.flush()

    @classmethod
    def tearDownClass(cls):
        super(EnvironmentTest, cls).tearDownClass()
        cls.compose_file.close()

    @data('events',
          'exec',
          'kill',
          'logs',
          'pause',
          'ps',
          'restart',
          'rm',
          'start',
          'stop',
          'top',
          'unpause')
    def _test_no_warning_on_missing_host_environment_var_on_silent_commands(self, cmd):
        options = {'COMMAND': cmd, '--file': [EnvironmentTest.compose_file.name]}
        with mock.patch('compose.config.environment.log') as fake_log:
            # Note that the warning silencing and the env variables check is
            # done in `project_from_options`
            # So no need to have a proper options map, the `COMMAND` key is enough
            project_from_options('.', options)
            assert fake_log.warn.call_count == 0


class EnvironmentOverrideFileTest(DockerClientTestCase):
    def test_env_file_override(self):
        base_dir = 'tests/fixtures/env-file-override'
        dispatch(base_dir, ['--env-file', '.env.override', 'up'])
        project = get_project(project_dir=base_dir,
                              config_path=['docker-compose.yml'],
                              environment=Environment.from_env_file(base_dir, '.env.override'),
                              override_dir=base_dir)
        containers = project.containers(stopped=True)
        assert len(containers) == 1
        assert "WHEREAMI=override" in containers[0].get('Config.Env')
        assert "DEFAULT_CONF_LOADED=true" in containers[0].get('Config.Env')
        dispatch(base_dir, ['--env-file', '.env.override', 'down'], None)
