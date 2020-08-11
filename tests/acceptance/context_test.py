import os
import shutil
import unittest

from docker import ContextAPI

from tests.acceptance.cli_test import dispatch


class ContextTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.docker_dir = os.path.join(os.environ.get("HOME", "/tmp"), '.docker')
        if not os.path.exists(cls.docker_dir):
            os.makedirs(cls.docker_dir)
        f = open(os.path.join(cls.docker_dir, "config.json"), "w")
        f.write("{}")
        f.close()
        cls.docker_config = os.path.join(cls.docker_dir, "config.json")
        os.environ['DOCKER_CONFIG'] = cls.docker_config
        ContextAPI.create_context("testcontext", host="tcp://doesnotexist:8000")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.docker_dir, ignore_errors=True)

    def setUp(self):
        self.base_dir = 'tests/fixtures/simple-composefile'
        self.override_dir = None

    def dispatch(self, options, project_options=None, returncode=0, stdin=None):
        return dispatch(self.base_dir, options, project_options, returncode, stdin)

    def test_help(self):
        result = self.dispatch(['help'], returncode=0)
        assert '-c, --context NAME' in result.stdout

    def test_fail_on_both_host_and_context_opt(self):
        result = self.dispatch(['-H', 'unix://', '-c', 'default', 'up'], returncode=1)
        assert '-H, --host and -c, --context are mutually exclusive' in result.stderr

    def test_fail_run_on_inexistent_context(self):
        result = self.dispatch(['-c', 'testcontext', 'up', '-d'], returncode=1)
        assert "Couldn't connect to Docker daemon" in result.stderr
