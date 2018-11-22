from __future__ import absolute_import
from __future__ import unicode_literals

import tempfile

from ddt import data
from ddt import ddt

from .. import mock
from compose.cli.command import project_from_options
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
    image: busybox:latest
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
